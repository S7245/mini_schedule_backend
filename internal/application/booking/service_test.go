package booking

import (
	"context"
	"testing"
	"time"

	domainbooking "github.com/zkw/mini-schedule/backend/internal/domain/booking"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	location     int64
	lastScope    []int64
	createCalled bool
	cancelCalled bool
	attendCalled bool
	endCalled    bool
	noShowCalled bool
}

func (r *fakeRepo) Create(_ context.Context, in domainbooking.CreateInput) (*domainbooking.Booking, error) {
	r.createCalled = true
	r.lastScope = in.ScopeLocationIDs
	return &domainbooking.Booking{ID: 1, LocationID: r.location, Status: domainbooking.StatusBooked}, nil
}
func (r *fakeRepo) Cancel(_ context.Context, _, _, id int64, _ string) (*domainbooking.Booking, error) {
	r.cancelCalled = true
	return &domainbooking.Booking{ID: id, Status: domainbooking.StatusCancelled}, nil
}
func (r *fakeRepo) List(_ context.Context, f domainbooking.ListFilter, _, _ int) ([]*domainbooking.Booking, int64, error) {
	r.lastScope = f.ScopeLocationIDs
	return []*domainbooking.Booking{}, 0, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domainbooking.Booking, error) {
	return &domainbooking.Booking{ID: id, LocationID: r.location, Status: domainbooking.StatusBooked}, nil
}
func (r *fakeRepo) UsableEntitlements(_ context.Context, _, _, _ int64, _ []int64) ([]*domainbooking.UsableEntitlement, error) {
	return nil, nil
}
func (r *fakeRepo) GetDefaultPolicy(_ context.Context, _ int64) (*domainbooking.Policy, error) {
	p := domainbooking.DefaultPolicy()
	return &p, nil
}
func (r *fakeRepo) UpsertDefaultPolicy(_ context.Context, _, _ int64, p domainbooking.Policy) (*domainbooking.Policy, error) {
	return &p, nil
}
func (r *fakeRepo) Attend(_ context.Context, _, _, id int64, _ string) (*domainbooking.Booking, error) {
	r.attendCalled = true
	return &domainbooking.Booking{ID: id, Status: domainbooking.StatusAttended}, nil
}
func (r *fakeRepo) EndSession(_ context.Context, _, _, sessionID int64, scope []int64) (*domainbooking.EndSessionResult, error) {
	r.endCalled = true
	r.lastScope = scope
	return &domainbooking.EndSessionResult{SessionID: sessionID, Status: "completed"}, nil
}
func (r *fakeRepo) EndSessionSystem(_ context.Context, sessionID int64) (*domainbooking.EndSessionResult, error) {
	return &domainbooking.EndSessionResult{SessionID: sessionID, Status: "completed"}, nil
}
func (r *fakeRepo) MarkSessionsInProgress(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *fakeRepo) ListDueSessionIDs(_ context.Context, _ time.Time) ([]int64, error) {
	return nil, nil
}
func (r *fakeRepo) ConfirmNoShow(_ context.Context, _, _, id int64, _ string) (*domainbooking.Booking, error) {
	r.noShowCalled = true
	return &domainbooking.Booking{ID: id, Status: domainbooking.StatusNoShow}, nil
}
func (r *fakeRepo) CreateByLearner(_ context.Context, _ domainbooking.LearnerCreateInput) (*domainbooking.Booking, error) {
	return &domainbooking.Booking{ID: 1, Source: domainbooking.SourceLearnerSelfService, Status: domainbooking.StatusBooked}, nil
}
func (r *fakeRepo) CancelByLearner(_ context.Context, _, _, id int64, _ string) (*domainbooking.Booking, error) {
	return &domainbooking.Booking{ID: id, Status: domainbooking.StatusCancelled}, nil
}

// captureChecker 记录 Require 收到的权限码，验证每个方法门的权限正确。
type captureChecker struct {
	scope domainrbac.DataScope
	codes []string
}

func (c *captureChecker) Require(_ context.Context, _, _ int64, code string) error {
	c.codes = append(c.codes, code)
	return nil
}
func (c *captureChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, c.scope, nil
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

func validCreate() CreateInput {
	return CreateInput{BrandID: 1, ActorID: 1, ClassSessionID: 10, BrandLearnerProfileID: 20, EntitlementMode: "auto"}
}

func TestCreate_InvalidMode(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := validCreate()
	in.EntitlementMode = "bogus"
	if _, err := s.Create(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_EmptyTargets(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := validCreate()
	in.ClassSessionID = 0
	if _, err := s.Create(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_PassesDataScope(t *testing.T) {
	repo := &fakeRepo{location: 7}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{7, 8}}})
	if _, err := s.Create(context.Background(), validCreate()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(repo.lastScope) != 2 || repo.lastScope[0] != 7 {
		t.Errorf("scope not passed to repo: %v", repo.lastScope)
	}
}

func TestCreate_RequireDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{})
	if _, err := s.Create(context.Background(), validCreate()); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.createCalled {
		t.Error("权限拒绝后不应调 repo.Create")
	}
}

func TestCancel_OutOfScope404(t *testing.T) {
	// booking 在 location 99，但 actor scope 只含 1 → 越权按不存在。
	repo := &fakeRepo{location: 99}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}})
	if _, err := s.Cancel(context.Background(), 1, 1, 5, "x"); codeOf(err) != apperr.ErrBookingNotFound {
		t.Fatalf("want BOOKING_NOT_FOUND, got %v", err)
	}
	if repo.cancelCalled {
		t.Error("越权后不应调 repo.Cancel")
	}
}

func TestCancel_InScopeOK(t *testing.T) {
	repo := &fakeRepo{location: 1}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}})
	if _, err := s.Cancel(context.Background(), 1, 1, 5, "x"); err != nil {
		t.Fatalf("in-scope cancel: %v", err)
	}
	if !repo.cancelCalled {
		t.Error("in-scope 应调 repo.Cancel")
	}
}

// ---- Batch 13e 签到 / 结束场次 / 爽约 ----

func TestAttendance_RequiresCorrectPermissions(t *testing.T) {
	repo := &fakeRepo{location: 1}
	chk := &captureChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAllBrand}}
	s := NewService(repo, chk)
	if _, err := s.Attend(context.Background(), 1, 1, 5, ""); err != nil {
		t.Fatalf("attend: %v", err)
	}
	if _, err := s.EndSession(context.Background(), 1, 1, 10); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, err := s.ConfirmNoShow(context.Background(), 1, 1, 5, ""); err != nil {
		t.Fatalf("no_show: %v", err)
	}
	want := []string{"attendance.mark", "attendance.mark", "attendance.no_show_confirm"}
	if len(chk.codes) != 3 || chk.codes[0] != want[0] || chk.codes[1] != want[1] || chk.codes[2] != want[2] {
		t.Errorf("required codes = %v, want %v", chk.codes, want)
	}
}

func TestAttend_OutOfScope404(t *testing.T) {
	repo := &fakeRepo{location: 99}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{1}}})
	if _, err := s.Attend(context.Background(), 1, 1, 5, ""); codeOf(err) != apperr.ErrBookingNotFound {
		t.Fatalf("want BOOKING_NOT_FOUND, got %v", err)
	}
	if repo.attendCalled {
		t.Error("越权后不应调 repo.Attend")
	}
}

func TestConfirmNoShow_RequireDenied(t *testing.T) {
	repo := &fakeRepo{location: 1}
	s := NewService(repo, denyChecker{})
	if _, err := s.ConfirmNoShow(context.Background(), 1, 1, 5, ""); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.noShowCalled {
		t.Error("权限拒绝后不应调 repo.ConfirmNoShow")
	}
}

func TestEndSession_PassesDataScope(t *testing.T) {
	repo := &fakeRepo{location: 7}
	s := NewService(repo, allowChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{7, 8}}})
	if _, err := s.EndSession(context.Background(), 1, 1, 10); err != nil {
		t.Fatalf("end: %v", err)
	}
	if len(repo.lastScope) != 2 || repo.lastScope[0] != 7 {
		t.Errorf("scope not passed to repo.EndSession: %v", repo.lastScope)
	}
}

func TestUpsertPolicy_NegativeRejected(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	if _, err := s.UpsertPolicy(context.Background(), 1, 1, domainbooking.Policy{CancelDeadlineMinutes: -5}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
	// 限额 0（非正）拒绝（不限请留 nil）。
	zero := 0
	if _, err := s.UpsertPolicy(context.Background(), 1, 1, domainbooking.Policy{DailyBookingLimit: &zero}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM for zero limit, got %v", err)
	}
}
