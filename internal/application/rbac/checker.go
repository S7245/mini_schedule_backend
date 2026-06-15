// Package rbac contains the application-layer permission checker.
//
// The Checker is injected into every brand-facing service (staff / location /
// brand profile / onboarding) and gates each method via Require(code). It
// resolves the caller's effective permission set with three cache tiers:
//
//  1. context-cache: same HTTP request reuses one Resolve via context.Value
//     (avoids N Redis hits when one request triggers multiple Require calls).
//  2. L1 cache (Redis 60s TTL via cacheStore): same brand_user across requests.
//  3. DB fallback: when cache is unavailable or misses, repo.LoadEffectiveRaw
//     runs the JOIN.
//
// is_owner=TRUE follows a fast-path: skip the assignment table entirely and
// grant every active permission + DataScopeAllBrand. This is mandatory — if
// the owner's role_assignments are misconfigured (which has happened in prod
// runbooks of similar systems), the fast-path prevents the brand from being
// self-locked.
package rbac

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// cacheTTL — short on purpose. Permission changes get picked up within ~1 min.
// Pub/sub invalidation is intentionally deferred (Grill node 10).
const cacheTTL = 60 * time.Second

const cacheKeyPrefix = "rbac:perms:"

// cachedResolve is the wire format for the L1 cache. Both fields are always
// populated (never nil) so JSON marshalling is deterministic.
type cachedResolve struct {
	Codes []string             `json:"codes"`
	Scope domainrbac.DataScope `json:"scope"`
}

// cacheStore is the Checker's L1 cache abstraction. Real impl wraps Redis;
// tests use an in-memory fake. Errors are intentionally non-fatal: Checker
// logs warnings and falls through to the DB on Get failures.
type cacheStore interface {
	Get(ctx context.Context, key string) (*cachedResolve, error)
	Set(ctx context.Context, key string, val *cachedResolve, ttl time.Duration) error
	// Del removes one or more keys in a single round-trip (Redis DEL is variadic).
	Del(ctx context.Context, keys ...string) error
}

// Checker is the central permission gate.
type Checker struct {
	repo  domainrbac.Repository
	cache cacheStore
	log   *slog.Logger

	allCatalogOnce sync.Once
	allCatalogErr  error
	allCatalog     []string
}

// NewChecker builds a Checker. logger may be nil (default discard logger used).
func NewChecker(repo domainrbac.Repository, cache cacheStore, log *slog.Logger) (*Checker, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return &Checker{repo: repo, cache: cache, log: log}, nil
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// Resolve returns the effective permission set + data scope for (brandID, brandUserID).
// Honours ctx-cache → L1 cache → repository, in that order. is_owner gets the
// fast-path of every active permission + DataScopeAllBrand.
func (c *Checker) Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	// 1) ctx-cache hit
	if rc := requestCacheGet(ctx, brandID, brandUserID); rc != nil {
		return domainrbac.Expand(rc.Codes), rc.Scope, nil
	}

	key := cacheKeyForUser(brandUserID)

	// 2) L1 cache hit
	if c.cache != nil {
		if got, err := c.cache.Get(ctx, key); err != nil {
			c.log.Warn("rbac cache get failed, falling back to DB",
				slog.Int64("brand_user_id", brandUserID),
				slog.Any("err", err),
			)
		} else if got != nil {
			requestCacheSet(ctx, brandID, brandUserID, got)
			return domainrbac.Expand(got.Codes), got.Scope, nil
		}
	}

	// 3) DB fallback via repository
	rawCodes, scope, isOwner, err := c.repo.LoadEffectiveRaw(ctx, brandID, brandUserID)
	if err != nil {
		return nil, domainrbac.DataScope{Kind: domainrbac.DataScopeNone}, err
	}

	codes := rawCodes
	effectiveScope := scope
	if isOwner {
		catalog, cerr := c.allActiveCodes(ctx)
		if cerr != nil {
			return nil, domainrbac.DataScope{Kind: domainrbac.DataScopeNone}, cerr
		}
		codes = catalog
		effectiveScope = domainrbac.DataScope{Kind: domainrbac.DataScopeAllBrand}
	}
	// Normalize: empty assignments → DataScopeNone (non-owner)
	if !isOwner && effectiveScope.Kind == "" {
		effectiveScope = domainrbac.DataScope{Kind: domainrbac.DataScopeNone}
	}

	rc := &cachedResolve{Codes: codes, Scope: effectiveScope}

	if c.cache != nil {
		if err := c.cache.Set(ctx, key, rc, cacheTTL); err != nil {
			c.log.Warn("rbac cache set failed",
				slog.Int64("brand_user_id", brandUserID),
				slog.Any("err", err),
			)
		}
	}
	requestCacheSet(ctx, brandID, brandUserID, rc)

	return domainrbac.Expand(rc.Codes), rc.Scope, nil
}

// Invalidate evicts the cached permission set for brandUserID. Call it after a
// role's permissions/status change (Batch 7 C1) for every affected brand_user so
// the next request re-resolves from DB instead of waiting out the 60s TTL.
//
// Both tiers are cleared: the L1 (Redis) key and any per-request ctx-cache entry
// matching this brandUserID. L1 Del failures are non-fatal (logged); the TTL is
// the backstop.
func (c *Checker) Invalidate(ctx context.Context, brandUserID int64) error {
	requestCacheDeleteUser(ctx, brandUserID)
	if c.cache == nil {
		return nil
	}
	if err := c.cache.Del(ctx, cacheKeyForUser(brandUserID)); err != nil {
		c.log.Warn("rbac cache del failed",
			slog.Int64("brand_user_id", brandUserID),
			slog.Any("err", err),
		)
		return err
	}
	return nil
}

// InvalidateMany evicts the cached permission sets for many brand_users in a
// single Redis DEL (instead of one round-trip per user). Call it after a role's
// permissions/status change to evict every holder at once. No-op on empty input.
//
// Both tiers are cleared per id: each per-request ctx-cache entry and, in one
// batched call, all L1 (Redis) keys. L1 Del failure is non-fatal (logged); the
// 60s TTL is the backstop.
func (c *Checker) InvalidateMany(ctx context.Context, brandUserIDs []int64) error {
	if len(brandUserIDs) == 0 {
		return nil
	}
	for _, id := range brandUserIDs {
		requestCacheDeleteUser(ctx, id)
	}
	if c.cache == nil {
		return nil
	}
	keys := make([]string, 0, len(brandUserIDs))
	for _, id := range brandUserIDs {
		keys = append(keys, cacheKeyForUser(id))
	}
	if err := c.cache.Del(ctx, keys...); err != nil {
		c.log.Warn("rbac cache del-many failed",
			slog.Int("count", len(keys)),
			slog.Any("err", err),
		)
		return err
	}
	return nil
}

// Require permits the action when the resolved set contains code, otherwise
// returns PERMISSION_DENIED with Details {required, missing}.
func (c *Checker) Require(ctx context.Context, brandID, brandUserID int64, code string) error {
	perms, _, err := c.Resolve(ctx, brandID, brandUserID)
	if err != nil {
		return err
	}
	if !perms.Has(code) {
		return apperr.NewAppError(apperr.ErrPermissionDenied, "权限不足", 403).
			WithDetails(map[string]any{
				"required": code,
				"missing":  []string{code},
			})
	}
	return nil
}

// RequireScope checks the caller's data scope against a target list of
// location_ids. Behaviour:
//
//   - AllBrand → always allow
//   - AssignedLocations + len(targets)==0 → allow (e.g. listing endpoints, scope is
//     applied via repository WHERE)
//   - AssignedLocations + non-empty targets → require non-empty intersection
//   - None → deny
func (c *Checker) RequireScope(ctx context.Context, brandID, brandUserID int64, targetLocationIDs []int64) error {
	_, scope, err := c.Resolve(ctx, brandID, brandUserID)
	if err != nil {
		return err
	}
	switch scope.Kind {
	case domainrbac.DataScopeAllBrand:
		return nil
	case domainrbac.DataScopeAssignedLocations:
		if len(targetLocationIDs) == 0 {
			return nil
		}
		allowed := make(map[int64]struct{}, len(scope.LocationIDs))
		for _, id := range scope.LocationIDs {
			allowed[id] = struct{}{}
		}
		for _, t := range targetLocationIDs {
			if _, ok := allowed[t]; ok {
				return nil
			}
		}
		return apperr.NewAppError(apperr.ErrPermissionDenied, "门店权限不足", 403).
			WithDetails(map[string]any{"required_scope": "assigned_locations"})
	default:
		return apperr.NewAppError(apperr.ErrPermissionDenied, "未配置数据权限", 403).
			WithDetails(map[string]any{"required_scope": "any"})
	}
}

func (c *Checker) allActiveCodes(ctx context.Context) ([]string, error) {
	c.allCatalogOnce.Do(func() {
		codes, err := c.repo.ListAllActivePermissionCodes(ctx)
		if err != nil {
			c.allCatalogErr = err
			return
		}
		sort.Strings(codes)
		c.allCatalog = codes
	})
	if c.allCatalogErr != nil {
		// reset Once so subsequent calls can retry
		c.allCatalogOnce = sync.Once{}
		return nil, c.allCatalogErr
	}
	return c.allCatalog, nil
}

func cacheKeyForUser(brandUserID int64) string {
	const base = cacheKeyPrefix
	return base + int64ToStr(brandUserID)
}

func int64ToStr(v int64) string {
	// avoid strconv import for hot path; small custom impl
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ---- ctx-cache (per-request reuse) -----------------------------------------

type ctxKeyType struct{}

var ctxKey ctxKeyType

type requestCache struct {
	mu   sync.Mutex
	data map[string]*cachedResolve
}

// WithRequestCache attaches a per-request cache to ctx. Middleware should call
// this at HTTP request boundary so the multiple RequirePermission calls within
// one request share a single Resolve result.
//
// Calling Resolve on a context without a request cache still works — it just
// won't dedup intra-request hits. The L1 cache (Redis) still applies.
func WithRequestCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(ctxKey).(*requestCache); ok {
		return ctx
	}
	return context.WithValue(ctx, ctxKey, &requestCache{data: map[string]*cachedResolve{}})
}

func requestCacheGet(ctx context.Context, brandID, brandUserID int64) *cachedResolve {
	rc, ok := ctx.Value(ctxKey).(*requestCache)
	if !ok || rc == nil {
		return nil
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	key := requestKey(brandID, brandUserID)
	if v, ok := rc.data[key]; ok && v != nil {
		out := *v
		return &out
	}
	return nil
}

func requestCacheSet(ctx context.Context, brandID, brandUserID int64, v *cachedResolve) {
	rc, ok := ctx.Value(ctxKey).(*requestCache)
	if !ok || rc == nil || v == nil {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	cp := *v
	rc.data[requestKey(brandID, brandUserID)] = &cp
}

func requestKey(brandID, brandUserID int64) string {
	return int64ToStr(brandID) + ":" + int64ToStr(brandUserID)
}

// requestCacheDeleteUser drops every ctx-cache entry for brandUserID regardless
// of brand (key suffix ":<brandUserID>"). No-op when ctx has no request cache.
func requestCacheDeleteUser(ctx context.Context, brandUserID int64) {
	rc, ok := ctx.Value(ctxKey).(*requestCache)
	if !ok || rc == nil {
		return
	}
	suffix := ":" + int64ToStr(brandUserID)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for k := range rc.data {
		if len(k) >= len(suffix) && k[len(k)-len(suffix):] == suffix {
			delete(rc.data, k)
		}
	}
}

// ---- Redis cache adapter ---------------------------------------------------

type redisCache struct {
	client *redis.Client
}

// NewRedisCacheStore returns a cacheStore backed by Redis.
func NewRedisCacheStore(client *redis.Client) cacheStore {
	if client == nil {
		return nil
	}
	return &redisCache{client: client}
}

func (r *redisCache) Get(ctx context.Context, key string) (*cachedResolve, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}
	raw, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var v cachedResolve
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *redisCache) Set(ctx context.Context, key string, val *cachedResolve, ttl time.Duration) error {
	if r == nil || r.client == nil || val == nil {
		return nil
	}
	buf, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, buf, ttl).Err()
}

func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	if r == nil || r.client == nil || len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}
