package recurringschedule

import (
	"context"
	"testing"
	"time"

	domainsession "github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainrec "github.com/zkw/mini-schedule/backend/internal/domain/recurringschedule"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	gotOccurrences int
	location       int64
}

func (r *fakeRepo) Generate(_ context.Context, in domainrec.GenerateInput) (*domainrec.GenerateResult, error) {
	r.gotOccurrences = len(in.Occurrences)
	return &domainrec.GenerateResult{Schedule: &domainrec.Schedule{ID: 1, LocationID: in.LocationID}}, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domainrec.Schedule, error) {
	return &domainrec.Schedule{ID: id, LocationID: r.location, Status: domainrec.StatusActive}, nil
}
func (r *fakeRepo) GetDetail(_ context.Context, _, id int64) (*domainrec.Schedule, []*domainsession.Session, error) {
	return &domainrec.Schedule{ID: id, LocationID: r.location}, nil, nil
}
func (r *fakeRepo) List(_ context.Context, _ domainrec.ListFilter, _, _ int) ([]*domainrec.Schedule, int64, error) {
	return nil, 0, nil
}
func (r *fakeRepo) Cancel(_ context.Context, _, _, id int64) (*domainrec.Schedule, error) {
	return &domainrec.Schedule{ID: id, Status: domainrec.StatusCancelled}, nil
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func futureDate(daysAhead int) string {
	d := time.Now().In(cstZone).AddDate(0, 0, daysAhead)
	return d.Format(dateLayout)
}

func baseInput() GenerateInput {
	rw := 4
	return GenerateInput{
		BrandID: 1, ActorID: 1, CourseID: 2, LocationID: 3, InstructorProfileID: 4,
		Weekdays: []int{1, 3}, StartDate: futureDate(7), RepeatWeeks: &rw,
		StartTime: "09:00", DurationMin: 60,
	}
}

func TestNormalizeWeekdays(t *testing.T) {
	if _, err := normalizeWeekdays(nil); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("empty weekdays should error, got %v", err)
	}
	if _, err := normalizeWeekdays([]int{7}); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("out-of-range weekday should error, got %v", err)
	}
	got, err := normalizeWeekdays([]int{3, 1, 3})
	if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("dedup+sort failed: %v %v", got, err)
	}
}

func TestGenerate_EndConditionXOR(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := baseInput()
	in.RepeatWeeks = nil
	in.EndDate = "" // neither
	if _, err := s.Generate(context.Background(), in); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("neither end condition should error, got %v", err)
	}
	in2 := baseInput()
	in2.EndDate = futureDate(30) // both repeat_weeks and end_date
	if _, err := s.Generate(context.Background(), in2); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("both end conditions should error, got %v", err)
	}
}

func TestGenerate_StartDateInPast(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := baseInput()
	in.StartDate = futureDate(-1)
	if _, err := s.Generate(context.Background(), in); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("past start_date should error, got %v", err)
	}
}

func TestGenerate_HorizonCap(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := baseInput()
	rw := 30 // 30 周 > 26 周
	in.RepeatWeeks = &rw
	if _, err := s.Generate(context.Background(), in); codeOf(err) != apperr.ErrSessionTimeInvalid {
		t.Fatalf("over-26-week horizon should error, got %v", err)
	}
}

func TestGenerate_DelegatesWithOccurrences(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)
	in := baseInput() // 周一+周三 × 4 周 = 8 节
	if _, err := s.Generate(context.Background(), in); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if repo.gotOccurrences != 8 {
		t.Fatalf("expected 8 occurrences (Mon+Wed × 4w), got %d", repo.gotOccurrences)
	}
}

func TestGenerate_OutOfScopeDenied(t *testing.T) {
	repo := &fakeRepo{}
	chk := scopeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{99}}}
	s := NewService(repo, chk)
	in := baseInput() // LocationID=3 不在 scope
	if _, err := s.Generate(context.Background(), in); codeOf(err) != apperr.ErrRecurringNotFound {
		t.Fatalf("out-of-scope should 404 RECURRING_NOT_FOUND, got %v", err)
	}
	if repo.gotOccurrences != 0 {
		t.Fatal("should not reach repo when out of scope")
	}
}

func TestBuildOccurrences_WeeklyOnSameWeekday(t *testing.T) {
	start := time.Date(2099, 1, 5, 0, 0, 0, 0, cstZone)
	wd := int(start.Weekday())
	end := start.AddDate(0, 0, 3*7-1) // 3 周
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	occ := buildOccurrences(start, end, []int{wd}, 9, 0, 60, past)
	if len(occ) != 3 {
		t.Fatalf("expected 3 weekly occurrences, got %d", len(occ))
	}
	if occ[0].TimeLabel != "09:00" {
		t.Fatalf("time label = %q", occ[0].TimeLabel)
	}
	// 每节间隔 7 天。
	if occ[1].StartsAt.Sub(occ[0].StartsAt) != 7*24*time.Hour {
		t.Fatalf("occurrences not 7 days apart")
	}
}

func TestBuildOccurrences_SkipsPast(t *testing.T) {
	start := time.Date(2099, 1, 5, 0, 0, 0, 0, cstZone)
	wd := int(start.Weekday())
	end := start.AddDate(0, 0, 7)
	// now 设在第二周之后 → 第一周那节被跳过。
	future := start.Add(8 * 24 * time.Hour).UTC()
	occ := buildOccurrences(start, end, []int{wd}, 9, 0, 60, future)
	if len(occ) != 0 {
		t.Fatalf("all occurrences are before now, expected 0, got %d", len(occ))
	}
}

// scopeChecker 全放行，data_scope 可配。
type scopeChecker struct{ scope domainrbac.DataScope }

func (scopeChecker) Require(_ context.Context, _, _ int64, _ string) error { return nil }
func (c scopeChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, c.scope, nil
}
