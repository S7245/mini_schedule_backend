package locationresource

import (
	"context"
	"testing"

	domainres "github.com/zkw/mini-schedule/backend/internal/domain/locationresource"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	created   bool
	deleted   bool
	location  int64
	createErr error
}

func (r *fakeRepo) Create(_ context.Context, in domainres.CreateInput) (*domainres.Resource, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.created = true
	return &domainres.Resource{ID: 1, LocationID: in.LocationID, Status: domainres.StatusActive}, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domainres.Resource, error) {
	return &domainres.Resource{ID: id, LocationID: r.location, Status: domainres.StatusActive}, nil
}
func (r *fakeRepo) List(_ context.Context, _ domainres.ListFilter, _, _ int) ([]*domainres.Resource, int64, error) {
	return []*domainres.Resource{}, 0, nil
}
func (r *fakeRepo) Update(_ context.Context, _, _, id int64, _ domainres.UpdateInput) (*domainres.Resource, error) {
	return &domainres.Resource{ID: id, LocationID: r.location}, nil
}
func (r *fakeRepo) Delete(_ context.Context, _, _, _ int64) error {
	r.deleted = true
	return nil
}

// denyChecker 拒绝指定 code，其余放行；data_scope all_brand。
type denyChecker struct{ deny string }

func (c denyChecker) Require(_ context.Context, _, _ int64, code string) error {
	if code == c.deny {
		return apperr.NewAppError(apperr.ErrForbidden, "权限不足", 403)
	}
	return nil
}
func (denyChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, domainrbac.DataScope{Kind: domainrbac.DataScopeAllBrand}, nil
}

// scopeChecker 全放行，data_scope 可配。
type scopeChecker struct{ scope domainrbac.DataScope }

func (scopeChecker) Require(_ context.Context, _, _ int64, _ string) error { return nil }
func (c scopeChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, c.scope, nil
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func validCreate(loc int64) CreateInput {
	return CreateInput{BrandID: 1, ActorID: 1, LocationID: loc, Name: "教室", Type: "classroom", Capacity: 5}
}

func TestCreate_PermissionDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{deny: "location_resource.create"})
	if _, err := s.Create(context.Background(), validCreate(5)); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.created {
		t.Fatal("should not reach repo when denied")
	}
}

func TestCreate_InvalidType(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := validCreate(5)
	in.Type = "spaceship"
	if _, err := s.Create(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_OutOfScopeDenied(t *testing.T) {
	repo := &fakeRepo{}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}}
	s := NewService(repo, chk)
	if _, err := s.Create(context.Background(), validCreate(9)); codeOf(err) != apperr.ErrResourceNotFound {
		t.Fatalf("want RESOURCE_NOT_FOUND (out-of-scope), got %v", err)
	}
	if repo.created {
		t.Fatal("should not reach repo when out of scope")
	}
}

func TestCreate_InScopeDelegates(t *testing.T) {
	repo := &fakeRepo{}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}}
	s := NewService(repo, chk)
	if _, err := s.Create(context.Background(), validCreate(5)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !repo.created {
		t.Fatal("should delegate to repo when in scope")
	}
}

func TestDelete_OutOfScope404(t *testing.T) {
	repo := &fakeRepo{location: 9}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}}
	s := NewService(repo, chk)
	if err := s.Delete(context.Background(), 1, 1, 1); codeOf(err) != apperr.ErrResourceNotFound {
		t.Fatalf("want RESOURCE_NOT_FOUND, got %v", err)
	}
	if repo.deleted {
		t.Fatal("should not delete when out of scope")
	}
}

func TestUpdate_InvalidStatus(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	bad := "paused"
	if _, err := s.Update(context.Background(), 1, 1, 1, UpdateInput{Status: &bad}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}
