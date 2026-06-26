package waitlist

import (
	"context"
	"testing"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainwaitlist "github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	location     int64
	lastScope    []int64
	joinCalled   bool
	promoteCalled bool
}

func (r *fakeRepo) Join(_ context.Context, in domainwaitlist.JoinInput) (*domainwaitlist.Entry, error) {
	r.joinCalled = true
	r.lastScope = in.ScopeLocationIDs
	return &domainwaitlist.Entry{ID: 1, LocationID: r.location, Status: domainwaitlist.StatusWaiting}, nil
}
func (r *fakeRepo) ListBySession(_ context.Context, _, _ int64, scope []int64) ([]*domainwaitlist.Entry, error) {
	r.lastScope = scope
	return nil, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domainwaitlist.Entry, error) {
	return &domainwaitlist.Entry{ID: id, LocationID: r.location, Status: domainwaitlist.StatusWaiting}, nil
}
func (r *fakeRepo) ListByLearner(_ context.Context, _, _ int64) ([]*domainwaitlist.Entry, error) {
	return nil, nil
}
func (r *fakeRepo) CancelByLearner(_ context.Context, _, _, id int64) (*domainwaitlist.Entry, error) {
	return &domainwaitlist.Entry{ID: id, Status: domainwaitlist.StatusCancelled}, nil
}
func (r *fakeRepo) Promote(_ context.Context, in domainwaitlist.PromoteInput) (*domainwaitlist.Entry, error) {
	r.promoteCalled = true
	return &domainwaitlist.Entry{ID: in.EntryID, Status: domainwaitlist.StatusPromoted}, nil
}
func (r *fakeRepo) Skip(_ context.Context, _, _, id int64, _ string) (*domainwaitlist.Entry, error) {
	return &domainwaitlist.Entry{ID: id, Status: domainwaitlist.StatusSkipped}, nil
}
func (r *fakeRepo) Cancel(_ context.Context, _, _, id int64) (*domainwaitlist.Entry, error) {
	return &domainwaitlist.Entry{ID: id, Status: domainwaitlist.StatusCancelled}, nil
}

type allowChecker struct{ scope domainrbac.DataScope }

func (allowChecker) Require(_ context.Context, _, _ int64, _ string) error { return nil }
func (c allowChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, c.scope, nil
}

type denyChecker struct{}

func (denyChecker) Require(_ context.Context, _, _ int64, _ string) error {
	return apperr.NewAppError(apperr.ErrForbidden, "无权限", 403)
}
func (denyChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, domainrbac.DataScope{}, nil
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func TestJoin_RequireDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{})
	if _, err := s.Join(context.Background(), 1, 1, 10, 20); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.joinCalled {
		t.Error("权限拒绝后不应调 repo.Join")
	}
}

func TestJoin_PassesScope(t *testing.T) {
	repo := &fakeRepo{location: 7}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{7, 8}}})
	if _, err := s.Join(context.Background(), 1, 1, 10, 20); err != nil {
		t.Fatalf("join: %v", err)
	}
	if len(repo.lastScope) != 2 || repo.lastScope[0] != 7 {
		t.Errorf("scope not passed: %v", repo.lastScope)
	}
}

func TestPromote_OutOfScope404(t *testing.T) {
	repo := &fakeRepo{location: 99}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}})
	if _, err := s.Promote(context.Background(), PromoteParams{BrandID: 1, ActorID: 1, EntryID: 5, EntitlementMode: "auto"}); codeOf(err) != apperr.ErrWaitlistEntryNotFound {
		t.Fatalf("want WAITLIST_ENTRY_NOT_FOUND, got %v", err)
	}
	if repo.promoteCalled {
		t.Error("越权后不应调 repo.Promote")
	}
}

func TestCancel_InScopeOK(t *testing.T) {
	repo := &fakeRepo{location: 1}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}})
	if _, err := s.Cancel(context.Background(), 1, 1, 5); err != nil {
		t.Fatalf("cancel: %v", err)
	}
}
