package persistence

import (
	"context"
	"time"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/internal/domain/training"

	"gorm.io/gorm"
)

type trainingRepository struct {
	db *gorm.DB
}

// NewTrainingRepository 创建训练记录仓储实现
func NewTrainingRepository(db *gorm.DB) training.Repository {
	return &trainingRepository{db: db}
}

func (r *trainingRepository) Create(ctx context.Context, input training.CreateRecordInput) (*training.Record, error) {
	m := TrainingRecordModel{
		UserID:      input.UserID,
		BrandID:     input.BrandID,
		CourseID:    input.CourseID,
		DurationMin: input.DurationMin,
		Calories:    input.Calories,
		Notes:       input.Notes,
		CompletedAt: time.Now(),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, apperr.ErrInternalF("创建训练记录失败", err)
	}
	return toTrainingDomain(&m), nil
}

func (r *trainingRepository) GetByID(ctx context.Context, id int64) (*training.Record, error) {
	var m TrainingRecordModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrTrainingNotFound, "训练记录不存在")
		}
		return nil, apperr.ErrInternalF("查询训练记录失败", err)
	}
	return toTrainingDomain(&m), nil
}

func (r *trainingRepository) ListByUserID(ctx context.Context, userID int64, offset, limit int) ([]*training.Record, int64, error) {
	var models []TrainingRecordModel
	var total int64

	query := r.db.WithContext(ctx).Model(&TrainingRecordModel{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询训练记录失败", err)
	}

	if err := query.Order("completed_at DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询训练记录失败", err)
	}

	items := make([]*training.Record, len(models))
	for i := range models {
		items[i] = toTrainingDomain(&models[i])
	}
	return items, total, nil
}

func (r *trainingRepository) ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*training.Record, int64, error) {
	var models []TrainingRecordModel
	var total int64

	query := r.db.WithContext(ctx).Model(&TrainingRecordModel{}).Where("brand_id = ?", brandID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询训练记录失败", err)
	}

	if err := query.Order("completed_at DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询训练记录失败", err)
	}

	items := make([]*training.Record, len(models))
	for i := range models {
		items[i] = toTrainingDomain(&models[i])
	}
	return items, total, nil
}

func toTrainingDomain(m *TrainingRecordModel) *training.Record {
	return &training.Record{
		ID:          m.ID,
		UserID:      m.UserID,
		BrandID:     m.BrandID,
		CourseID:    m.CourseID,
		DurationMin: m.DurationMin,
		Calories:    m.Calories,
		Notes:       m.Notes,
		CompletedAt: m.CompletedAt,
		CreatedAt:   m.CreatedAt,
	}
}
