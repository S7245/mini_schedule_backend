package onboarding

import (
	"context"
	"strings"
	"time"

	domainonboarding "github.com/zkw/mini-schedule/backend/internal/domain/onboarding"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// Service onboarding 应用服务，编排 GET status / Skip / Complete。
type Service struct {
	repo domainonboarding.Repository
}

// NewService 创建 onboarding 应用服务。
func NewService(repo domainonboarding.Repository) *Service {
	return &Service{repo: repo}
}

// GetOnboardingStatus 聚合 brand 信息 + 7 张表 COUNT + 已存的 step 记录，
// 实时计算每个 step 的 status：
//
//   - 已 SkipStep 标记的非强制步骤 → skipped；
//   - 业务计数判定 completed → completed（覆盖 not_started）；
//   - 否则保持已存值，缺失则 not_started。
//
// brand.status != active → 403 BRAND_NOT_ACTIVE。
func (s *Service) GetOnboardingStatus(ctx context.Context, brandID int64) (*domainonboarding.OnboardingStatus, error) {
	if brandID <= 0 {
		return nil, apperr.ErrBadRequest("品牌 ID 无效")
	}

	summary, err := s.repo.GetBrandSummary(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if summary.Status != "active" {
		return nil, apperr.NewAppError(apperr.ErrBrandNotActive, "品牌未激活", 403)
	}

	stepRecords, err := s.repo.GetSteps(ctx, brandID)
	if err != nil {
		return nil, err
	}
	stepMap := make(map[domainonboarding.StepKey]domainonboarding.StepRecord, len(stepRecords))
	for _, r := range stepRecords {
		stepMap[r.StepKey] = r
	}

	counts, err := s.repo.GetCounts(ctx, brandID)
	if err != nil {
		return nil, err
	}

	steps := make([]domainonboarding.StepView, 0, len(domainonboarding.AllSteps()))
	var nextKey *domainonboarding.StepKey

	for _, key := range domainonboarding.AllSteps() {
		view := computeStepView(key, summary, counts, stepMap)
		steps = append(steps, view)
		if nextKey == nil && view.Status != domainonboarding.StepStatusCompleted && view.Status != domainonboarding.StepStatusSkipped {
			k := key
			nextKey = &k
		}
	}

	overall := computeOverallStatus(summary, steps)

	return &domainonboarding.OnboardingStatus{
		OverallStatus:         overall,
		Steps:                 steps,
		NextStepKey:           nextKey,
		OnboardingCompletedAt: summary.OnboardingCompletedAt,
	}, nil
}

// SkipStep 跳过非强制 step。brand_profile / location 调用一律 STEP_NOT_SKIPPABLE。
func (s *Service) SkipStep(ctx context.Context, brandID int64, stepKey string, reason string) (*domainonboarding.StepRecord, error) {
	if brandID <= 0 {
		return nil, apperr.ErrBadRequest("品牌 ID 无效")
	}
	if !domainonboarding.IsValidStepKey(stepKey) {
		return nil, apperr.NewAppError(apperr.ErrInvalidStepKey, "无效的 step key", 400)
	}
	key := domainonboarding.StepKey(stepKey)
	if !domainonboarding.IsSkippable(key) {
		return nil, apperr.NewAppError(apperr.ErrStepNotSkippable, "该步骤不允许跳过", 400)
	}

	summary, err := s.repo.GetBrandSummary(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if summary.Status != "active" {
		return nil, apperr.NewAppError(apperr.ErrBrandNotActive, "品牌未激活", 403)
	}

	return s.repo.UpsertSkippedStep(ctx, brandID, key, strings.TrimSpace(reason))
}

// Complete 校验所有 8 步处于 completed / skipped，否则 ONBOARDING_NOT_READY。
// 重复调用幂等：第二次直接返已 completed 视图。
func (s *Service) Complete(ctx context.Context, brandID int64) (*domainonboarding.OnboardingStatus, error) {
	if brandID <= 0 {
		return nil, apperr.ErrBadRequest("品牌 ID 无效")
	}

	status, err := s.GetOnboardingStatus(ctx, brandID)
	if err != nil {
		return nil, err
	}

	if status.OverallStatus == domainonboarding.OverallStatusCompleted {
		return status, nil
	}

	pending := make([]string, 0)
	for _, v := range status.Steps {
		if v.Status != domainonboarding.StepStatusCompleted && v.Status != domainonboarding.StepStatusSkipped {
			pending = append(pending, string(v.StepKey))
		}
	}
	if len(pending) > 0 {
		msg := "尚有未完成步骤: " + strings.Join(pending, ",")
		return nil, apperr.NewAppError(apperr.ErrOnboardingNotReady, msg, 400)
	}

	now := time.Now().UTC()
	if err := s.repo.MarkBrandOnboardingCompleted(ctx, brandID, now); err != nil {
		return nil, err
	}
	// 清空 step metadata（per 契约 Q4）
	if err := s.repo.ClearAllStepMetadata(ctx, brandID); err != nil {
		return nil, err
	}

	status.OverallStatus = domainonboarding.OverallStatusCompleted
	status.OnboardingCompletedAt = &now
	status.NextStepKey = nil
	return status, nil
}

// computeStepView 单步实时计算逻辑。
func computeStepView(
	key domainonboarding.StepKey,
	summary *domainonboarding.BrandSummary,
	counts *domainonboarding.CountsByStep,
	stepMap map[domainonboarding.StepKey]domainonboarding.StepRecord,
) domainonboarding.StepView {
	target := int64(1)
	var count int64
	completed := false

	switch key {
	case domainonboarding.StepBrandProfile:
		// description + industry_type 均非空 → completed
		if strings.TrimSpace(summary.Description) != "" && strings.TrimSpace(summary.IndustryType) != "" {
			completed = true
			count = 1
		}
	case domainonboarding.StepLocation:
		count = counts.Location
		completed = count >= target
	case domainonboarding.StepStaff:
		// staff: active brand_users ≥ 1 AND instructor_profiles ≥ 1
		// count 字段返回二者最小值作为前端进度展示
		if counts.Staff > 0 && counts.InstructorProfile > 0 {
			completed = true
		}
		count = minInt64(counts.Staff, counts.InstructorProfile)
	case domainonboarding.StepCourseCategory:
		count = counts.CourseCategory
		completed = count >= target
	case domainonboarding.StepCourseTemplate:
		count = counts.CourseTemplate
		completed = count >= target
	case domainonboarding.StepEntitlementTemplate:
		count = counts.EntitlementTemplate
		completed = count >= target
	case domainonboarding.StepClassSession:
		count = counts.ClassSession
		completed = count >= target
	case domainonboarding.StepMiniProgramQRCode:
		// 本批未实做：当前 mini program 未建表，count 恒为 0，只能 skip。
		count = 0
	}

	rec, hasRec := stepMap[key]
	view := domainonboarding.StepView{
		StepKey: key,
		Count:   count,
		Target:  target,
	}

	switch {
	case completed:
		view.Status = domainonboarding.StepStatusCompleted
		if hasRec && rec.CompletedAt != nil {
			view.CompletedAt = rec.CompletedAt
		}
	case hasRec && rec.Status == domainonboarding.StepStatusSkipped && domainonboarding.IsSkippable(key):
		view.Status = domainonboarding.StepStatusSkipped
		view.SkippedAt = rec.SkippedAt
	case hasRec && rec.Status == domainonboarding.StepStatusInProgress:
		view.Status = domainonboarding.StepStatusInProgress
	default:
		view.Status = domainonboarding.StepStatusNotStarted
	}

	return view
}

func computeOverallStatus(summary *domainonboarding.BrandSummary, steps []domainonboarding.StepView) domainonboarding.OverallStatus {
	if summary.OnboardingStatus == "completed" {
		return domainonboarding.OverallStatusCompleted
	}
	hasAny := false
	for _, v := range steps {
		if v.Status == domainonboarding.StepStatusCompleted || v.Status == domainonboarding.StepStatusSkipped {
			hasAny = true
			break
		}
	}
	if hasAny {
		return domainonboarding.OverallStatusInProgress
	}
	return domainonboarding.OverallStatusNotStarted
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

