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
	// 清掉历史 completed_at，避免出现 status='skipped' 但 completed_at 非空的精分行（review #5）。
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "brand_id"}, {Name: "step_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":       string(onboarding.StepStatusSkipped),
			"skipped_at":   now,
			"completed_at": nil,
			"updated_at":   now,
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

// CompleteOnboarding 单事务内同时：
//  1. UPDATE brands SET onboarding_status='completed', onboarding_completed_at=?
//  2. UPDATE brand_onboarding_steps SET metadata='{}' WHERE brand_id=?
//
// review #1：原先两步跨 TX，中间失败 + retry 会因 idempotent fast-path 跳过 ClearAllStepMetadata。
func (r *onboardingRepository) CompleteOnboarding(ctx context.Context, brandID int64, completedAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&BrandModel{}).
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
		if err := tx.Model(&BrandOnboardingStepModel{}).
			Where("brand_id = ?", brandID).
			Update("metadata", []byte("{}")).Error; err != nil {
			return apperr.ErrInternalF("清空 onboarding 步骤元数据失败", err)
		}
		return nil
	})
}

// EnsureStepCompleted upsert 给定 keys 的 brand_onboarding_steps 行为 status='completed'。
// ON CONFLICT DO UPDATE 但**仅当**当前 status NOT IN ('skipped') —— 保留用户主动 skip 的选择。
// 设计说明（review #3）：GetOnboardingStatus 在 in-memory 算出 completed 后调用本方法持久化 completed_at，
// 避免视图层永远返回 completed_at=NULL；下游审计/分析直接读表也能看到真实的完成时间。
func (r *onboardingRepository) EnsureStepCompleted(
	ctx context.Context, brandID int64, keys []onboarding.StepKey, completedAt time.Time,
) (map[onboarding.StepKey]time.Time, error) {
	out := make(map[onboarding.StepKey]time.Time, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, k := range keys {
			m := BrandOnboardingStepModel{
				BrandID:     brandID,
				StepKey:     string(k),
				Status:      string(onboarding.StepStatusCompleted),
				CompletedAt: &completedAt,
				Metadata:    []byte("{}"),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "brand_id"}, {Name: "step_key"}},
				// 只有当前不是 skipped 才覆盖（保留用户的主动 skip 决定）。
				// 已是 completed 的不更新 completed_at（保留首次完成时间）。
				Where: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "brand_onboarding_steps.status NOT IN ('skipped','completed')"},
				}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"status":       string(onboarding.StepStatusCompleted),
					"completed_at": completedAt,
					"updated_at":   completedAt,
				}),
			}).Create(&m).Error; err != nil {
				return apperr.ErrInternalF("持久化 onboarding completed 失败", err)
			}
			// 读回一次拿到真实的 completed_at（已存在的旧 completed 行不会被覆盖，时间是旧的）
			var got BrandOnboardingStepModel
			if err := tx.Where("brand_id = ? AND step_key = ?", brandID, string(k)).First(&got).Error; err != nil {
				return apperr.ErrInternalF("查询 onboarding 步骤失败", err)
			}
			if got.CompletedAt != nil {
				out[k] = *got.CompletedAt
			}
		}
		return nil
	})
	return out, err
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
	// 状态过滤策略（review #4，与各表的 status check constraint 对齐）：
	//   - locations:            排除软删（无 status 过滤；onboarding 只要存在过有效门店即算）
	//   - brand_users:          active（已是；忽略 disabled 员工）
	//   - instructor_profiles:  active（排除 inactive 教练）
	//   - course_categories:    active
	//   - courses:              排除软删 + published（Batch 11 CourseTemplate 发布态；排除 draft / archived）
	//   - entitlement_products: active
	//   - class_sessions:       scheduled / in_progress / completed（排除 draft / cancelled）
	queries := []kv{
		{&out.Location, "SELECT COUNT(*) FROM locations WHERE brand_id = ? AND deleted_at IS NULL"},
		{&out.Staff, "SELECT COUNT(*) FROM brand_users WHERE brand_id = ? AND deleted_at IS NULL AND status = 'active'"},
		{&out.InstructorProfile, "SELECT COUNT(*) FROM instructor_profiles WHERE brand_id = ? AND status = 'active'"},
		{&out.CourseCategory, "SELECT COUNT(*) FROM course_categories WHERE brand_id = ? AND status = 'active'"},
		{&out.CourseTemplate, "SELECT COUNT(*) FROM courses WHERE brand_id = ? AND deleted_at IS NULL AND status = 'published'"},
		{&out.EntitlementTemplate, "SELECT COUNT(*) FROM entitlement_products WHERE brand_id = ? AND status = 'active'"},
		{&out.ClassSession, "SELECT COUNT(*) FROM class_sessions WHERE brand_id = ? AND status IN ('scheduled', 'in_progress', 'completed')"},
	}

	for _, q := range queries {
		if err := db.Raw(q.sql, brandID).Scan(q.field).Error; err != nil {
			return nil, apperr.ErrInternalF("聚合 onboarding 计数失败", err)
		}
	}
	return out, nil
}
