package rbac

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// ---- fakes ----

type fakeRBACRepo struct {
	loadCalls   int32
	loadErr     error
	rawCodes    []string
	scope       domainrbac.DataScope
	isOwner     bool
	listAllErr  error
	allCodes    []string
}

func (f *fakeRBACRepo) LoadEffectiveRaw(_ context.Context, _ int64, _ int64) ([]string, domainrbac.DataScope, bool, error) {
	atomic.AddInt32(&f.loadCalls, 1)
	return f.rawCodes, f.scope, f.isOwner, f.loadErr
}

func (f *fakeRBACRepo) ListAllActivePermissionCodes(_ context.Context) ([]string, error) {
	return f.allCodes, f.listAllErr
}

type memCache struct {
	mu     sync.Mutex
	store  map[string]cacheEntry
	getErr error
	setErr error
}

func newMemCache() *memCache {
	return &memCache{store: map[string]cacheEntry{}}
}

func (c *memCache) Get(_ context.Context, key string) (*cachedResolve, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return nil, c.getErr
	}
	e, ok := c.store[key]
	if !ok {
		return nil, nil
	}
	if time.Now().After(e.expiresAt) {
		delete(c.store, key)
		return nil, nil
	}
	out := e.val
	return &out, nil
}

func (c *memCache) Set(_ context.Context, key string, val *cachedResolve, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	c.store[key] = cacheEntry{val: *val, expiresAt: time.Now().Add(time.Hour)}
	return nil
}

type cacheEntry struct {
	val       cachedResolve
	expiresAt time.Time
}

// ---- tests ----

func newCheckerWithFakes(t *testing.T, repo *fakeRBACRepo, cache cacheStore) *Checker {
	t.Helper()
	c, err := NewChecker(repo, cache, nil)
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}
	return c
}

func TestRequire_Permits(t *testing.T) {
	repo := &fakeRBACRepo{rawCodes: []string{"staff.edit"}, scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	if err := c.Require(context.Background(), 1, 18, "staff.view"); err != nil {
		t.Fatalf("expected staff.view (implied from staff.edit), got %v", err)
	}
}

func TestRequire_Denies(t *testing.T) {
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}, scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	err := c.Require(context.Background(), 1, 18, "staff.create")
	ae := apperr.GetAppError(err)
	if ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
	if ae.HTTPStatus != 403 {
		t.Fatalf("expected 403, got %d", ae.HTTPStatus)
	}
	if ae.Details["required"] != "staff.create" {
		t.Fatalf("expected required=staff.create, got %#v", ae.Details)
	}
}

func TestResolve_CachesAcrossCalls(t *testing.T) {
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}}
	cache := newMemCache()
	c := newCheckerWithFakes(t, repo, cache)
	ctx := context.Background()
	if _, _, err := c.Resolve(ctx, 1, 18); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Resolve(ctx, 1, 18); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&repo.loadCalls); got != 1 {
		t.Fatalf("repo should be hit once thanks to cache, got %d calls", got)
	}
}

func TestResolve_CtxCacheReusesWithinRequest(t *testing.T) {
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	ctx := WithRequestCache(context.Background())

	// First call → loads
	if _, _, err := c.Resolve(ctx, 1, 18); err != nil {
		t.Fatal(err)
	}
	// Second call within the same ctx should reuse the ctx cache, not even
	// touch the L1 cache (memCache is also empty initially but we want to
	// guarantee the same cachedResolve pointer reuse).
	calls := atomic.LoadInt32(&repo.loadCalls)
	if _, _, err := c.Resolve(ctx, 1, 18); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&repo.loadCalls); got != calls {
		t.Fatalf("ctx cache should suppress repo hits, got %d (was %d)", got, calls)
	}
}

func TestResolve_OwnerFastPathFromCatalog(t *testing.T) {
	repo := &fakeRBACRepo{
		isOwner:  true,
		allCodes: []string{"staff.create", "staff.delete", "location.view", "brand.profile.edit"},
	}
	c := newCheckerWithFakes(t, repo, newMemCache())
	ps, ds, err := c.Resolve(context.Background(), 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Kind != domainrbac.DataScopeAllBrand {
		t.Fatalf("owner should get DataScopeAllBrand, got %v", ds.Kind)
	}
	// Implied propagation: staff.delete → staff.edit + staff.view
	if !ps.HasAll("staff.create", "staff.delete", "staff.edit", "staff.view", "brand.profile.edit", "brand.profile.view", "location.view") {
		t.Fatalf("owner missing implied perms, got %v", ps.Codes())
	}
}

func TestResolve_OwnerWithoutAnyRoleAssignment(t *testing.T) {
	// E29: owner with zero role assignments still has full power.
	repo := &fakeRBACRepo{isOwner: true, allCodes: []string{"staff.view", "staff.create"}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	if err := c.Require(context.Background(), 1, 16, "staff.create"); err != nil {
		t.Fatalf("owner should bypass missing assignments, got %v", err)
	}
}

func TestResolve_NoAssignmentsForNonOwner(t *testing.T) {
	repo := &fakeRBACRepo{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeNone}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	err := c.Require(context.Background(), 1, 18, "staff.view")
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED for empty perms, got %v", err)
	}
}

func TestResolve_CacheGetFailureFallsBackToDB(t *testing.T) {
	// E27: Redis unreachable → checker still works
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}}
	cache := newMemCache()
	cache.getErr = errors.New("redis down")
	cache.setErr = errors.New("redis down")
	c := newCheckerWithFakes(t, repo, cache)
	ctx := context.Background()
	if err := c.Require(ctx, 1, 18, "staff.view"); err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if err := c.Require(ctx, 1, 18, "staff.view"); err != nil {
		t.Fatalf("expected fallback success second call, got %v", err)
	}
	// Repo hit twice because cache always errors out
	if got := atomic.LoadInt32(&repo.loadCalls); got != 2 {
		t.Fatalf("expected repo hit per call when cache fails, got %d", got)
	}
}

func TestRequireScope_AllBrandAllowsAll(t *testing.T) {
	repo := &fakeRBACRepo{isOwner: true, allCodes: []string{"staff.view"}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	if err := c.RequireScope(context.Background(), 1, 16, []int64{99}); err != nil {
		t.Fatalf("owner all_brand should allow any location, got %v", err)
	}
}

func TestRequireScope_AssignedLocationsAllowsIntersection(t *testing.T) {
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}, scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1, 2}}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	if err := c.RequireScope(context.Background(), 1, 18, []int64{2, 5}); err != nil {
		t.Fatalf("intersection should pass, got %v", err)
	}
}

func TestRequireScope_NoIntersectionDenies(t *testing.T) {
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}, scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	err := c.RequireScope(context.Background(), 1, 18, []int64{2, 3})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestRequireScope_NoTargetsTreatedAsScopeOnly(t *testing.T) {
	// If caller passes no target location_ids, RequireScope should still pass
	// for AllBrand and AssignedLocations alike (they want a generic listing).
	repo := &fakeRBACRepo{rawCodes: []string{"staff.view"}, scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	if err := c.RequireScope(context.Background(), 1, 18, nil); err != nil {
		t.Fatalf("nil target should not deny, got %v", err)
	}
}

func TestRequireScope_NoneDenies(t *testing.T) {
	repo := &fakeRBACRepo{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeNone}}
	c := newCheckerWithFakes(t, repo, newMemCache())
	err := c.RequireScope(context.Background(), 1, 18, []int64{1})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}
