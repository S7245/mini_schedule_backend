package learnerbooking

import (
	"context"
	"testing"

	domainbooking "github.com/zkw/mini-schedule/backend/internal/domain/booking"
	domainsession "github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	domainent "github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	domainwaitlist "github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// 嵌入接口的部分 fake：未覆盖方法 promote 自 nil 接口（不调用即安全）。
type fakeBookingRepo struct {
	domainbooking.Repository
	createIn       domainbooking.LearnerCreateInput
	listFilter     domainbooking.ListFilter
	cancelArgs     [3]int64 // brand, profile, id
	usableArgs     [3]int64 // brand, session, learner
	usableScopeNil bool
}

func (f *fakeBookingRepo) CreateByLearner(_ context.Context, in domainbooking.LearnerCreateInput) (*domainbooking.Booking, error) {
	f.createIn = in
	return &domainbooking.Booking{ID: 1, Source: domainbooking.SourceLearnerSelfService, Status: domainbooking.StatusBooked}, nil
}
func (f *fakeBookingRepo) List(_ context.Context, filter domainbooking.ListFilter, _, _ int) ([]*domainbooking.Booking, int64, error) {
	f.listFilter = filter
	return nil, 0, nil
}
func (f *fakeBookingRepo) CancelByLearner(_ context.Context, brandID, profileID, id int64, _ string) (*domainbooking.Booking, error) {
	f.cancelArgs = [3]int64{brandID, profileID, id}
	return &domainbooking.Booking{ID: id, Status: domainbooking.StatusCancelled}, nil
}
func (f *fakeBookingRepo) UsableEntitlements(_ context.Context, brandID, sessionID, learnerID int64, scope []int64) ([]*domainbooking.UsableEntitlement, error) {
	f.usableArgs = [3]int64{brandID, sessionID, learnerID}
	f.usableScopeNil = scope == nil
	return nil, nil
}

type fakeSessionRepo struct {
	domainsession.Repository
	listFilter domainsession.ListFilter
}

func (f *fakeSessionRepo) List(_ context.Context, filter domainsession.ListFilter, _, _ int) ([]*domainsession.Session, int64, error) {
	f.listFilter = filter
	return nil, 0, nil
}
func (f *fakeSessionRepo) GetByID(_ context.Context, _, id int64) (*domainsession.Session, error) {
	return &domainsession.Session{ID: id}, nil
}

type fakeEntRepo struct {
	domainent.Repository
	listArgs [2]int64 // brand, learner
}

func (f *fakeEntRepo) ListEntitlementsByLearner(_ context.Context, brandID, learnerID int64) ([]*domainent.Entitlement, error) {
	f.listArgs = [2]int64{brandID, learnerID}
	return nil, nil
}

type fakeWaitlistRepo struct {
	domainwaitlist.Repository
	joinIn     domainwaitlist.JoinInput
	listArgs   [2]int64 // brand, profile
	cancelArgs [3]int64 // brand, profile, id
}

func (f *fakeWaitlistRepo) Join(_ context.Context, in domainwaitlist.JoinInput) (*domainwaitlist.Entry, error) {
	f.joinIn = in
	return &domainwaitlist.Entry{ID: 1, Status: domainwaitlist.StatusWaiting}, nil
}
func (f *fakeWaitlistRepo) ListByLearner(_ context.Context, brandID, profileID int64) ([]*domainwaitlist.Entry, error) {
	f.listArgs = [2]int64{brandID, profileID}
	return nil, nil
}
func (f *fakeWaitlistRepo) CancelByLearner(_ context.Context, brandID, profileID, id int64) (*domainwaitlist.Entry, error) {
	f.cancelArgs = [3]int64{brandID, profileID, id}
	return &domainwaitlist.Entry{ID: id, Status: domainwaitlist.StatusCancelled}, nil
}

func assertCode(t *testing.T, err error, want apperr.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s, got nil", want)
	}
	ae, ok := err.(*apperr.AppError)
	if !ok || ae.Code != want {
		t.Fatalf("expected %s, got %T %v", want, err, err)
	}
}

func TestLearnerBookingService_ListSessions_ScheduledFuture(t *testing.T) {
	sr := &fakeSessionRepo{}
	svc := NewService(&fakeBookingRepo{}, sr, &fakeEntRepo{}, &fakeWaitlistRepo{})
	if _, _, err := svc.ListSessions(context.Background(), 21, 1, 20); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if sr.listFilter.BrandID != 21 || sr.listFilter.Status != "scheduled" {
		t.Errorf("filter brand=%d status=%q, want 21/scheduled", sr.listFilter.BrandID, sr.listFilter.Status)
	}
	if sr.listFilter.From == nil {
		t.Error("From should be set (future only)")
	}
	if sr.listFilter.ScopeLocationIDs != nil {
		t.Error("learner has no data_scope; ScopeLocationIDs must be nil")
	}
}

func TestLearnerBookingService_Book_PassesProfile(t *testing.T) {
	br := &fakeBookingRepo{}
	svc := NewService(br, &fakeSessionRepo{}, &fakeEntRepo{}, &fakeWaitlistRepo{})
	if _, err := svc.Book(context.Background(), 21, 99, 5); err != nil {
		t.Fatalf("Book: %v", err)
	}
	if br.createIn.BrandID != 21 || br.createIn.BrandLearnerProfileID != 99 || br.createIn.ClassSessionID != 5 {
		t.Errorf("createIn = %+v, want brand21/profile99/session5", br.createIn)
	}
}

func TestLearnerBookingService_ListMyBookings_OwnProfileOnly(t *testing.T) {
	br := &fakeBookingRepo{}
	svc := NewService(br, &fakeSessionRepo{}, &fakeEntRepo{}, &fakeWaitlistRepo{})
	if _, _, err := svc.ListMyBookings(context.Background(), 21, 99, "booked", 1, 20); err != nil {
		t.Fatalf("ListMyBookings: %v", err)
	}
	if br.listFilter.BrandLearnerProfileID != 99 {
		t.Errorf("must filter own profile, got %d", br.listFilter.BrandLearnerProfileID)
	}
	if len(br.listFilter.Statuses) != 1 || br.listFilter.Statuses[0] != "booked" {
		t.Errorf("statuses filter=%v, want [booked]", br.listFilter.Statuses)
	}
	if br.listFilter.ScopeLocationIDs != nil {
		t.Error("learner has no data_scope")
	}
}

// TestLearnerBookingService_ListMyBookings_MultiStatus 上课记录：逗号分隔 → Statuses（14b）。
func TestLearnerBookingService_ListMyBookings_MultiStatus(t *testing.T) {
	br := &fakeBookingRepo{}
	svc := NewService(br, &fakeSessionRepo{}, &fakeEntRepo{}, &fakeWaitlistRepo{})
	if _, _, err := svc.ListMyBookings(context.Background(), 21, 99, "attended,no_show", 1, 20); err != nil {
		t.Fatalf("ListMyBookings multi: %v", err)
	}
	if len(br.listFilter.Statuses) != 2 || br.listFilter.Statuses[0] != "attended" || br.listFilter.Statuses[1] != "no_show" {
		t.Errorf("statuses=%v, want [attended no_show]", br.listFilter.Statuses)
	}
}

// TestLearnerBookingService_Entitlements_Waitlist 14b 增量：权益/候补按本 profile 收口。
func TestLearnerBookingService_Entitlements_Waitlist(t *testing.T) {
	er := &fakeEntRepo{}
	wr := &fakeWaitlistRepo{}
	svc := NewService(&fakeBookingRepo{}, &fakeSessionRepo{}, er, wr)
	ctx := context.Background()

	if _, err := svc.ListMyEntitlements(ctx, 21, 99); err != nil {
		t.Fatalf("ListMyEntitlements: %v", err)
	}
	if er.listArgs != [2]int64{21, 99} {
		t.Errorf("entitlement listArgs=%v, want [21 99]", er.listArgs)
	}

	if _, err := svc.JoinWaitlist(ctx, 21, 99, 5); err != nil {
		t.Fatalf("JoinWaitlist: %v", err)
	}
	if wr.joinIn.BrandID != 21 || wr.joinIn.BrandLearnerProfileID != 99 || wr.joinIn.ClassSessionID != 5 || !wr.joinIn.SelfService {
		t.Errorf("joinIn=%+v, want brand21/profile99/session5/SelfService", wr.joinIn)
	}

	if _, err := svc.ListMyWaitlist(ctx, 21, 99); err != nil {
		t.Fatalf("ListMyWaitlist: %v", err)
	}
	if wr.listArgs != [2]int64{21, 99} {
		t.Errorf("waitlist listArgs=%v, want [21 99]", wr.listArgs)
	}

	if _, err := svc.CancelMyWaitlist(ctx, 21, 99, 7); err != nil {
		t.Fatalf("CancelMyWaitlist: %v", err)
	}
	if wr.cancelArgs != [3]int64{21, 99, 7} {
		t.Errorf("waitlist cancelArgs=%v, want [21 99 7]", wr.cancelArgs)
	}
}

// TestLearnerBookingService_RequireProfile_14b 14b 方法 profile_id=0 → 401，不触 repo。
func TestLearnerBookingService_RequireProfile_14b(t *testing.T) {
	er := &fakeEntRepo{}
	wr := &fakeWaitlistRepo{}
	svc := NewService(&fakeBookingRepo{}, &fakeSessionRepo{}, er, wr)
	ctx := context.Background()
	if _, err := svc.ListMyEntitlements(ctx, 21, 0); err == nil {
		t.Error("ListMyEntitlements should 401 on profile 0")
	}
	if _, err := svc.JoinWaitlist(ctx, 21, 0, 5); err == nil {
		t.Error("JoinWaitlist should 401 on profile 0")
	}
	if er.listArgs[0] != 0 || wr.joinIn.BrandID != 0 {
		t.Error("repo must not be called when profile missing")
	}
}

func TestLearnerBookingService_Cancel_PassesProfile(t *testing.T) {
	br := &fakeBookingRepo{}
	svc := NewService(br, &fakeSessionRepo{}, &fakeEntRepo{}, &fakeWaitlistRepo{})
	if _, err := svc.CancelMyBooking(context.Background(), 21, 99, 7, "x"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if br.cancelArgs != [3]int64{21, 99, 7} {
		t.Errorf("cancelArgs=%v, want [21 99 7]", br.cancelArgs)
	}
}

func TestLearnerBookingService_UsableEntitlements_ScopeNil(t *testing.T) {
	br := &fakeBookingRepo{}
	svc := NewService(br, &fakeSessionRepo{}, &fakeEntRepo{}, &fakeWaitlistRepo{})
	if _, err := svc.UsableEntitlements(context.Background(), 21, 99, 5); err != nil {
		t.Fatalf("UsableEntitlements: %v", err)
	}
	if !br.usableScopeNil {
		t.Error("scope must be nil (no data_scope)")
	}
	if br.usableArgs != [3]int64{21, 5, 99} { // brand, session, learner
		t.Errorf("usableArgs=%v, want [21 5 99]", br.usableArgs)
	}
}

// TestLearnerBookingService_RequireProfile 旧 token（profile_id=0）→ 401，且不触 repo。
func TestLearnerBookingService_RequireProfile(t *testing.T) {
	br := &fakeBookingRepo{}
	svc := NewService(br, &fakeSessionRepo{}, &fakeEntRepo{}, &fakeWaitlistRepo{})
	_, err := svc.Book(context.Background(), 21, 0, 5)
	assertCode(t, err, apperr.ErrUnauthorized)
	if br.createIn.BrandID != 0 {
		t.Error("repo must not be called when profile missing")
	}
	_, _, err = svc.ListMyBookings(context.Background(), 21, 0, "", 1, 20)
	assertCode(t, err, apperr.ErrUnauthorized)
	_, err = svc.CancelMyBooking(context.Background(), 21, 0, 7, "")
	assertCode(t, err, apperr.ErrUnauthorized)
	_, err = svc.UsableEntitlements(context.Background(), 21, 0, 5)
	assertCode(t, err, apperr.ErrUnauthorized)
}
