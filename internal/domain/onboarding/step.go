package onboarding

import (
	"context"
	"time"
)

// StepKey 向导步骤唯一标识。
type StepKey string

const (
	StepBrandProfile        StepKey = "brand_profile"
	StepLocation            StepKey = "location"
	StepStaff               StepKey = "staff"
	StepCourseCategory      StepKey = "course_category"
	StepCourseTemplate      StepKey = "course_template"
	StepEntitlementTemplate StepKey = "entitlement_template"
	StepClassSession        StepKey = "class_session"
	StepMiniProgramQRCode   StepKey = "mini_program_qrcode"
)

// AllSteps 按蓝图固定顺序的 8 步。
func AllSteps() []StepKey {
	return []StepKey{
		StepBrandProfile,
		StepLocation,
		StepStaff,
		StepCourseCategory,
		StepCourseTemplate,
		StepEntitlementTemplate,
		StepClassSession,
		StepMiniProgramQRCode,
	}
}

// IsValidStepKey 判断字符串是否对应合法的 step key。
func IsValidStepKey(key string) bool {
	for _, s := range AllSteps() {
		if string(s) == key {
			return true
		}
	}
	return false
}

// IsSkippable 第 1-2 步禁止跳过。
func IsSkippable(key StepKey) bool {
	switch key {
	case StepBrandProfile, StepLocation:
		return false
	}
	return true
}

// StepStatus 步骤状态枚举。
type StepStatus string

const (
	StepStatusNotStarted StepStatus = "not_started"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusSkipped    StepStatus = "skipped"
)

// OverallStatus 品牌 onboarding 总状态。
type OverallStatus string

const (
	OverallStatusNotStarted OverallStatus = "not_started"
	OverallStatusInProgress OverallStatus = "in_progress"
	OverallStatusCompleted  OverallStatus = "completed"
)

// StepRecord 是 brand_onboarding_steps 表的领域投影。
type StepRecord struct {
	BrandID     int64
	StepKey     StepKey
	Status      StepStatus
	CompletedAt *time.Time
	SkippedAt   *time.Time
}

// StepView 是返回给上层 / 前端的单步视图。
type StepView struct {
	StepKey     StepKey    `json:"step_key"`
	Status      StepStatus `json:"status"`
	CompletedAt *time.Time `json:"completed_at"`
	SkippedAt   *time.Time `json:"skipped_at"`
	Count       int64      `json:"count"`
	Target      int64      `json:"target"`
}

// OnboardingStatus 是 GET /onboarding/status 的聚合视图模型。
type OnboardingStatus struct {
	OverallStatus         OverallStatus `json:"overall_status"`
	Steps                 []StepView    `json:"steps"`
	NextStepKey           *StepKey      `json:"next_step_key"`
	OnboardingCompletedAt *time.Time    `json:"onboarding_completed_at"`
}

// BrandSummary 是 status 计算需要的 brand 侧只读字段。
type BrandSummary struct {
	ID                    int64
	Status                string
	Description           string
	IndustryType          string
	OnboardingStatus      string
	OnboardingCompletedAt *time.Time
}

// CountsByStep 是 7 张资源表的 COUNT 聚合，给 service 用于实时计算 status。
type CountsByStep struct {
	Location            int64
	Staff               int64
	InstructorProfile   int64
	CourseCategory      int64
	CourseTemplate      int64
	EntitlementTemplate int64
	ClassSession        int64
}

// Repository onboarding 仓储接口。
type Repository interface {
	GetBrandSummary(ctx context.Context, brandID int64) (*BrandSummary, error)
	GetSteps(ctx context.Context, brandID int64) ([]StepRecord, error)
	UpsertSkippedStep(ctx context.Context, brandID int64, key StepKey, reason string) (*StepRecord, error)
	MarkBrandOnboardingCompleted(ctx context.Context, brandID int64, completedAt time.Time) error
	ClearAllStepMetadata(ctx context.Context, brandID int64) error
	GetCounts(ctx context.Context, brandID int64) (*CountsByStep, error)
}
