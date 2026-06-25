package learner

import (
	"context"
	"testing"

	domainlearner "github.com/zkw/mini-schedule/backend/internal/domain/learner"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	created       bool
	deleted       bool
	statusUpdated bool
	tagCreated    bool
	location      *int64 // GetByID 返回的 profile 主门店。
}

func (r *fakeRepo) Create(_ context.Context, in domainlearner.CreateInput) (*domainlearner.Profile, error) {
	r.created = true
	return &domainlearner.Profile{ID: 1, PrimaryLocationID: in.PrimaryLocationID, Status: domainlearner.StatusActive}, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domainlearner.Profile, error) {
	return &domainlearner.Profile{ID: id, PrimaryLocationID: r.location, Status: domainlearner.StatusActive}, nil
}
func (r *fakeRepo) FindOrCreateProfileByOpenID(_ context.Context, _ int64, _, _ string) (*domainlearner.Profile, error) {
	return &domainlearner.Profile{ID: 1, Status: domainlearner.StatusActive}, nil
}
func (r *fakeRepo) List(_ context.Context, _ domainlearner.ListFilter, _, _ int) ([]*domainlearner.Profile, int64, error) {
	return []*domainlearner.Profile{}, 0, nil
}
func (r *fakeRepo) Update(_ context.Context, _, _, id int64, _ domainlearner.UpdateInput) (*domainlearner.Profile, error) {
	return &domainlearner.Profile{ID: id, PrimaryLocationID: r.location}, nil
}
func (r *fakeRepo) UpdateStatus(_ context.Context, _, _, id int64, status string) (*domainlearner.Profile, error) {
	r.statusUpdated = true
	return &domainlearner.Profile{ID: id, Status: domainlearner.Status(status)}, nil
}
func (r *fakeRepo) Delete(_ context.Context, _, _, _ int64) error {
	r.deleted = true
	return nil
}
func (r *fakeRepo) CreateTag(_ context.Context, in domainlearner.CreateTagInput) (*domainlearner.Tag, error) {
	r.tagCreated = true
	return &domainlearner.Tag{ID: 1, Name: in.Name, Status: domainlearner.TagStatusActive}, nil
}
func (r *fakeRepo) ListTags(_ context.Context, _ domainlearner.TagListFilter) ([]*domainlearner.Tag, error) {
	return []*domainlearner.Tag{}, nil
}
func (r *fakeRepo) UpdateTag(_ context.Context, _, _, id int64, in domainlearner.UpdateTagInput) (*domainlearner.Tag, error) {
	return &domainlearner.Tag{ID: id}, nil
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

func assignedScope(ids ...int64) scopeChecker {
	return scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: ids}}
}

func ptr64(v int64) *int64 { return &v }

func TestCreate_PermissionDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{deny: "learner.create"})
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 1, Phone: "13700000000"})
	if codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.created {
		t.Fatal("should not reach repo when denied")
	}
}

func TestCreate_InvalidPhone(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	if _, err := s.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 1, Phone: "  "}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_AssignedScopeRequiresPrimaryLocation(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, assignedScope(5))
	// 无主门店 → assigned 员工被拒。
	if _, err := s.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 1, Phone: "13700000000"}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM (must specify primary location), got %v", err)
	}
	if repo.created {
		t.Fatal("should not reach repo")
	}
}

func TestCreate_OutOfScopePrimaryLocationDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, assignedScope(5))
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 1, Phone: "13700000000", PrimaryLocationID: ptr64(9)})
	if codeOf(err) != apperr.ErrLearnerNotFound {
		t.Fatalf("want LEARNER_NOT_FOUND (out-of-scope), got %v", err)
	}
	if repo.created {
		t.Fatal("should not reach repo when out of scope")
	}
}

func TestCreate_InScopeDelegates(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, assignedScope(5))
	if _, err := s.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 1, Phone: "13700000000", PrimaryLocationID: ptr64(5)}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !repo.created {
		t.Fatal("should delegate to repo when in scope")
	}
}

func TestGet_OutOfScope404(t *testing.T) {
	repo := &fakeRepo{location: ptr64(9)}
	s := NewService(repo, assignedScope(5))
	if _, err := s.Get(context.Background(), 1, 1, 1); codeOf(err) != apperr.ErrLearnerNotFound {
		t.Fatalf("want LEARNER_NOT_FOUND, got %v", err)
	}
}

func TestDelete_OutOfScope404(t *testing.T) {
	repo := &fakeRepo{location: ptr64(9)}
	s := NewService(repo, assignedScope(5))
	if err := s.Delete(context.Background(), 1, 1, 1); codeOf(err) != apperr.ErrLearnerNotFound {
		t.Fatalf("want LEARNER_NOT_FOUND, got %v", err)
	}
	if repo.deleted {
		t.Fatal("should not delete when out of scope")
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)
	// freeze 端点只接受 active/frozen，inactive 非法。
	if _, err := s.UpdateStatus(context.Background(), 1, 1, 1, "inactive"); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
	if repo.statusUpdated {
		t.Fatal("should not update with invalid status")
	}
}

func TestCreateTag_PermissionDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{deny: "learner.edit"})
	if _, err := s.CreateTag(context.Background(), CreateTagInput{BrandID: 1, ActorID: 1, Name: "VIP"}); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.tagCreated {
		t.Fatal("should not reach repo when denied")
	}
}

func TestCreateTag_EmptyName(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	if _, err := s.CreateTag(context.Background(), CreateTagInput{BrandID: 1, ActorID: 1, Name: "  "}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}
