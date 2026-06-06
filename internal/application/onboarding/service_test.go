package onboarding

import (
	"context"
	"testing"
	"time"

	domainonboarding "github.com/zkw/mini-schedule/backend/internal/domain/onboarding"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// fakeRepo 是手写的 in-memory repository，足够覆盖 service 单测路径。
type fakeRepo struct {
	summary       *domainonboarding.BrandSummary
	summaryErr    error
	steps         []domainonboarding.StepRecord
	stepsErr      error
	counts        *domainonboarding.CountsByStep
	countsErr     error
	upsertCalls   []upsertCall
	markedAt      *time.Time
	clearCalled   bool
	markErr       error
	upsertErr     error
}

type upsertCall struct {
	BrandID int64
	Key     domainonboarding.StepKey
	Reason  string
}

func (r *fakeRepo) GetBrandSummary(_ context.Context, _ int64) (*domainonboarding.BrandSummary, error) {
	return r.summary, r.summaryErr
}

func (r *fakeRepo) GetSteps(_ context.Context, _ int64) ([]domainonboarding.StepRecord, error) {
	return r.steps, r.stepsErr
}

func (r *fakeRepo) UpsertSkippedStep(_ context.Context, brandID int64, key domainonboarding.StepKey, reason string) (*domainonboarding.StepRecord, error) {
	if r.upsertErr != nil {
		return nil, r.upsertErr
	}
	r.upsertCalls = append(r.upsertCalls, upsertCall{brandID, key, reason})
	now := time.Now().UTC()
	return &domainonboarding.StepRecord{
		BrandID:   brandID,
		StepKey:   key,
		Status:    domainonboarding.StepStatusSkipped,
		SkippedAt: &now,
	}, nil
}

func (r *fakeRepo) CompleteOnboarding(_ context.Context, _ int64, t time.Time) error {
	if r.markErr != nil {
		return r.markErr
	}
	r.markedAt = &t
	r.clearCalled = true // 单事务里 metadata 也清掉
	return nil
}

func (r *fakeRepo) GetCounts(_ context.Context, _ int64) (*domainonboarding.CountsByStep, error) {
	return r.counts, r.countsErr
}

func (r *fakeRepo) EnsureStepCompleted(
	_ context.Context, _ int64, keys []domainonboarding.StepKey, t time.Time,
) (map[domainonboarding.StepKey]time.Time, error) {
	out := make(map[domainonboarding.StepKey]time.Time, len(keys))
	for _, k := range keys {
		out[k] = t
	}
	return out, nil
}

func newActiveSummary() *domainonboarding.BrandSummary {
	return &domainonboarding.BrandSummary{
		ID:               1,
		Status:           "active",
		OnboardingStatus: "in_progress",
	}
}

func TestGetOnboardingStatus_BrandNotActive(t *testing.T) {
	repo := &fakeRepo{
		summary: &domainonboarding.BrandSummary{ID: 1, Status: "pending"},
	}
	svc := NewService(repo)
	_, err := svc.GetOnboardingStatus(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for pending brand")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrBrandNotActive {
		t.Fatalf("expected BRAND_NOT_ACTIVE, got %v", err)
	}
}

func TestGetOnboardingStatus_AllNotStarted(t *testing.T) {
	repo := &fakeRepo{
		summary: newActiveSummary(),
		counts:  &domainonboarding.CountsByStep{},
	}
	svc := NewService(repo)
	st, err := svc.GetOnboardingStatus(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Steps) != 8 {
		t.Fatalf("want 8 steps, got %d", len(st.Steps))
	}
	for _, s := range st.Steps {
		if s.Status != domainonboarding.StepStatusNotStarted {
			t.Errorf("step %s want not_started, got %s", s.StepKey, s.Status)
		}
	}
	if st.NextStepKey == nil || *st.NextStepKey != domainonboarding.StepBrandProfile {
		t.Errorf("next should be brand_profile, got %v", st.NextStepKey)
	}
	if st.OverallStatus != domainonboarding.OverallStatusNotStarted {
		t.Errorf("overall not_started expected, got %s", st.OverallStatus)
	}
}

func TestGetOnboardingStatus_BrandProfileCompletedByContent(t *testing.T) {
	repo := &fakeRepo{
		summary: &domainonboarding.BrandSummary{
			ID: 1, Status: "active",
			Description:  "x",
			IndustryType: "fitness",
		},
		counts: &domainonboarding.CountsByStep{Location: 1},
	}
	svc := NewService(repo)
	st, err := svc.GetOnboardingStatus(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Steps[0].Status != domainonboarding.StepStatusCompleted {
		t.Errorf("brand_profile should be completed, got %s", st.Steps[0].Status)
	}
	if st.Steps[1].Status != domainonboarding.StepStatusCompleted {
		t.Errorf("location should be completed, got %s", st.Steps[1].Status)
	}
	// next should be staff
	if st.NextStepKey == nil || *st.NextStepKey != domainonboarding.StepStaff {
		t.Errorf("next should be staff, got %v", st.NextStepKey)
	}
}

func TestGetOnboardingStatus_StaffRequiresBothCounts(t *testing.T) {
	repo := &fakeRepo{
		summary: newActiveSummary(),
		counts:  &domainonboarding.CountsByStep{Staff: 2, InstructorProfile: 0},
	}
	svc := NewService(repo)
	st, _ := svc.GetOnboardingStatus(context.Background(), 1)
	// staff should NOT be completed
	if st.Steps[2].Status == domainonboarding.StepStatusCompleted {
		t.Error("staff should not complete when instructor_profiles=0")
	}
}

func TestGetOnboardingStatus_SkippedPrevails(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		summary: newActiveSummary(),
		counts:  &domainonboarding.CountsByStep{},
		steps: []domainonboarding.StepRecord{
			{StepKey: domainonboarding.StepStaff, Status: domainonboarding.StepStatusSkipped, SkippedAt: &now},
		},
	}
	svc := NewService(repo)
	st, _ := svc.GetOnboardingStatus(context.Background(), 1)
	if st.Steps[2].Status != domainonboarding.StepStatusSkipped {
		t.Errorf("staff should reflect skipped, got %s", st.Steps[2].Status)
	}
}

func TestSkipStep_BlocksMandatorySteps(t *testing.T) {
	repo := &fakeRepo{summary: newActiveSummary()}
	svc := NewService(repo)
	for _, k := range []string{"brand_profile", "location"} {
		_, err := svc.SkipStep(context.Background(), 1, k, "")
		if err == nil {
			t.Errorf("expected error skipping %s", k)
			continue
		}
		ae := apperr.GetAppError(err)
		if ae == nil || ae.Code != apperr.ErrStepNotSkippable {
			t.Errorf("expected STEP_NOT_SKIPPABLE for %s, got %v", k, err)
		}
	}
}

func TestSkipStep_InvalidKey(t *testing.T) {
	repo := &fakeRepo{summary: newActiveSummary()}
	svc := NewService(repo)
	_, err := svc.SkipStep(context.Background(), 1, "foo", "")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidStepKey {
		t.Errorf("expected INVALID_STEP_KEY, got %v", err)
	}
}

func TestSkipStep_HappyPath(t *testing.T) {
	repo := &fakeRepo{summary: newActiveSummary()}
	svc := NewService(repo)
	rec, err := svc.SkipStep(context.Background(), 1, "staff", "not now")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != domainonboarding.StepStatusSkipped {
		t.Errorf("expected skipped, got %s", rec.Status)
	}
	if len(repo.upsertCalls) != 1 {
		t.Errorf("expected 1 upsert call, got %d", len(repo.upsertCalls))
	}
	if repo.upsertCalls[0].Reason != "not now" {
		t.Errorf("reason not propagated")
	}
}

func TestComplete_NotReady(t *testing.T) {
	repo := &fakeRepo{
		summary: newActiveSummary(),
		counts:  &domainonboarding.CountsByStep{},
	}
	svc := NewService(repo)
	_, err := svc.Complete(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when steps incomplete")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrOnboardingNotReady {
		t.Errorf("expected ONBOARDING_NOT_READY, got %v", err)
	}
}

func TestComplete_AllSkippedOrDone(t *testing.T) {
	now := time.Now().UTC()
	steps := []domainonboarding.StepRecord{}
	for _, k := range []domainonboarding.StepKey{
		domainonboarding.StepStaff,
		domainonboarding.StepCourseCategory,
		domainonboarding.StepCourseTemplate,
		domainonboarding.StepEntitlementTemplate,
		domainonboarding.StepClassSession,
		domainonboarding.StepMiniProgramQRCode,
	} {
		steps = append(steps, domainonboarding.StepRecord{
			StepKey: k, Status: domainonboarding.StepStatusSkipped, SkippedAt: &now,
		})
	}
	repo := &fakeRepo{
		summary: &domainonboarding.BrandSummary{
			ID: 1, Status: "active",
			Description:  "x",
			IndustryType: "y",
		},
		counts: &domainonboarding.CountsByStep{Location: 1},
		steps:  steps,
	}
	svc := NewService(repo)
	st, err := svc.Complete(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if st.OverallStatus != domainonboarding.OverallStatusCompleted {
		t.Errorf("overall should be completed")
	}
	if repo.markedAt == nil {
		t.Errorf("MarkBrandOnboardingCompleted not called")
	}
	if !repo.clearCalled {
		t.Errorf("ClearAllStepMetadata not called")
	}
}

func TestComplete_Idempotent(t *testing.T) {
	completedAt := time.Now().UTC()
	repo := &fakeRepo{
		summary: &domainonboarding.BrandSummary{
			ID: 1, Status: "active",
			Description:           "x",
			IndustryType:          "y",
			OnboardingStatus:      "completed",
			OnboardingCompletedAt: &completedAt,
		},
		counts: &domainonboarding.CountsByStep{Location: 1},
	}
	svc := NewService(repo)
	st, err := svc.Complete(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if st.OverallStatus != domainonboarding.OverallStatusCompleted {
		t.Errorf("idempotent complete should still report completed")
	}
	if repo.markedAt != nil {
		t.Errorf("idempotent path should not re-mark complete")
	}
}
