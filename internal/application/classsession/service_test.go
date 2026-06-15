package classsession

import (
	"context"
	"testing"
	"time"

	domainsession "github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	created     bool
	location    int64
	createErr   error
}

func (r *fakeRepo) Create(_ context.Context, in domainsession.CreateInput) (*domainsession.Session, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.created = true
	return &domainsession.Session{ID: 1, LocationID: in.LocationID, Status: domainsession.StatusScheduled}, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domainsession.Session, error) {
	return &domainsession.Session{ID: id, LocationID: r.location, Status: domainsession.StatusScheduled}, nil
}
func (r *fakeRepo) List(_ context.Context, _ domainsession.ListFilter, _, _ int) ([]*domainsession.Session, int64, error) {
	return []*domainsession.Session{}, 0, nil
}
func (r *fakeRepo) Cancel(_ context.Context, _, _, id int64, _ string) (*domainsession.Session, error) {
	return &domainsession.Session{ID: id, Status: domainsession.StatusCancelled}, nil
}

// scopeChecker 允许所有权限，data_scope 可配。
type scopeChecker struct {
	scope domainrbac.DataScope
}

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
	return CreateInput{
		BrandID: 1, ActorID: 1, CourseID: 2, LocationID: loc, InstructorProfileID: 3,
		StartsAt: time.Now().UTC().Add(24 * time.Hour),
		EndsAt:   time.Now().UTC().Add(25 * time.Hour),
		Capacity: 8,
	}
}

func TestCreate_EndsBeforeStarts(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := validCreate(5)
	in.EndsAt = in.StartsAt.Add(-time.Hour)
	if _, err := s.Create(context.Background(), in); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("want SESSION_TIME_INVALID, got %v", err)
	}
}

func TestCreate_StartsInPast(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := validCreate(5)
	in.StartsAt = time.Now().UTC().Add(-time.Hour)
	in.EndsAt = time.Now().UTC().Add(time.Hour)
	if _, err := s.Create(context.Background(), in); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("want SESSION_TIME_INVALID, got %v", err)
	}
}

func TestCreate_OutOfScopeLocationDenied(t *testing.T) {
	repo := &fakeRepo{}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}}
	s := NewService(repo, chk)
	if _, err := s.Create(context.Background(), validCreate(9)); codeOf(err) != apperr.ErrSessionNotFound {
		t.Fatalf("want SESSION_NOT_FOUND (out-of-scope), got %v", err)
	}
	if repo.created {
		t.Fatal("should not reach repo when location out of scope")
	}
}

func TestCreate_InScopeLocationDelegates(t *testing.T) {
	repo := &fakeRepo{}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}}
	s := NewService(repo, chk)
	if _, err := s.Create(context.Background(), validCreate(5)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !repo.created {
		t.Fatal("should delegate to repo when location in scope")
	}
}

func TestGet_OutOfScope404(t *testing.T) {
	repo := &fakeRepo{location: 9}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}}
	s := NewService(repo, chk)
	if _, err := s.Get(context.Background(), 1, 1, 1); codeOf(err) != apperr.ErrSessionNotFound {
		t.Fatalf("want SESSION_NOT_FOUND, got %v", err)
	}
}
