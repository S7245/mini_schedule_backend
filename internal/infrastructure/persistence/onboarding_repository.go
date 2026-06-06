package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/domain/onboarding"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type onboardingRepository struct {
	db *gorm.DB
}

// NewOnboardingRepository 创建 Onboarding 仓储。
func NewOnboardingRepository(db *gorm.DB) onboarding.Repository {
	return &onboardingRepository{db: db}
}

func (r *onboardingRepository) GetBrandSummary(ctx context.Context, brandID int64) (*onboarding.BrandSummary, error) {
	var m BrandModel
	if err := r.db.WithContext(ctx).Where("id = ?", brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
		}
		return nil, apperr.ErrInternalF("查询品牌失败", err)
	}
	return &onboarding.BrandSummary{
		ID:                    m.ID,
		Status:                m.Status,
		Description:           m.Description,
		IndustryType:          m.IndustryType,
		OnboardingStatus:      m.OnboardingStatus,
		OnboardingCompletedAt: m.OnboardingCompletedAt,
	}, nil
}

func (r *onboardingRepository) GetSteps(ctx context.Context, brandID int64) ([]onboarding.StepRecord, error) {
	var ms []BrandOnboardingStepModel
	if err := r.db.WithContext(ctx).
		Where("brand_id = ?", brandID).
		Find(&ms).Error; err != nil {
		return nil, apperr.ErrInternalF("查询 onboarding 步骤失败", err)
	}
	out := make([]onboarding.StepRecord, 0, len(ms))
	for i := range ms {
		out = append(out, onboarding.StepRecord{
			BrandID:     ms[i].BrandID,
			StepKey:     onboarding.StepKey(ms[i].StepKey),
			Status:      onboarding.StepStatus(ms[i].Status),
			CompletedAt: ms[i].CompletedAt,
			SkippedAt:   ms[i].SkippedAt,
		})
	}
	return out, nil
}

// UpsertSkippedStep 将指定 step 置为 skipped；存在则 UPDATE，不存在 INSERT。
func (r *onboardingRepository) UpsertSkippedStep(ctx context.Context, brandID int64, key onboarding.StepKey, reason string) (*onboarding.StepRecord, error) {
	now := time.Now().UTC()
	m := BrandOnboardingStepModel{
		BrandID:   brandID,
		StepKey:   string(key),
		Status:    string(onboarding.StepStatusSkipped),
		SkippedAt: &now,
		Metadata:  []byte("{}"),
	}

	// ON CONFLICT (brand_id, step_key) DO UPDATE
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "brand_id"}, {Name: "step_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":     string(onboarding.StepStatusSkipped),
			"skipped_at": now,
			"updated_at": now,
		}),
	}).Create(&m).Error; err != nil {
		return nil, apperr.ErrInternalF("写入 onboarding skip 失败", err)
	}

	return &onboarding.StepRecord{
		BrandID:   brandID,
		StepKey:   key,
		Status:    onboarding.StepStatusSkipped,
		SkippedAt: &now,
	}, nil
}

func (r *onboardingRepository) MarkBrandOnboardingCompleted(ctx context.Context, brandID int64, completedAt time.Time) error {
	res := r.db.WithContext(ctx).Model(&BrandModel{}).
		Where("id = ?", brandID).
		Updates(map[string]interface{}{
			"onboarding_status":       "completed",
			"onboarding_completed_at": completedAt,
		})
	if res.Error != nil {
		return apperr.ErrInternalF("标记 onboarding 完成失败", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
	}
	return nil
}

func (r *onboardingRepository) ClearAllStepMetadata(ctx context.Context, brandID int64) error {
	if err := r.db.WithContext(ctx).Model(&BrandOnboardingStepModel{}).
		Where("brand_id = ?", brandID).
		Update("metadata", []byte("{}")).Error; err != nil {
		return apperr.ErrInternalF("清空 onboarding 步骤元数据失败", err)
	}
	return nil
}

// GetCounts 聚合 7 张资源表的 COUNT。
// brand_users / instructor_profiles / course_categories / entitlement_products / class_sessions
// 在 Batch 4 内尚未有 GORM 模型，使用 raw SQL；locations 复用模型。
func (r *onboardingRepository) GetCounts(ctx context.Context, brandID int64) (*onboarding.CountsByStep, error) {
	out := &onboarding.CountsByStep{}
	db := r.db.WithContext(ctx)

	type kv struct {
		field *int64
		sql   string
	}
	queries := []kv{
		{&out.Location, "SELECT COUNT(*) FROM locations WHERE brand_id = ? AND deleted_at IS NULL"},
		{&out.Staff, "SELECT COUNT(*) FROM brand_users WHERE brand_id = ? AND deleted_at IS NULL AND status = 'active'"},
		{&out.InstructorProfile, "SELECT COUNT(*) FROM instructor_profiles WHERE brand_id = ? AND deleted_at IS NULL"},
		{&out.CourseCategory, "SELECT COUNT(*) FROM course_categories WHERE brand_id = ?"},
		{&out.CourseTemplate, "SELECT COUNT(*) FROM courses WHERE brand_id = ? AND deleted_at IS NULL"},
		{&out.EntitlementTemplate, "SELECT COUNT(*) FROM entitlement_products WHERE brand_id = ?"},
		{&out.ClassSession, "SELECT COUNT(*) FROM class_sessions WHERE brand_id = ?"},
	}

	for _, q := range queries {
		if err := db.Raw(q.sql, brandID).Scan(q.field).Error; err != nil {
			return nil, apperr.ErrInternalF("聚合 onboarding 计数失败", err)
		}
	}
	return out, nil
}
