package persistence

import (
	"context"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/internal/domain/course"

	"gorm.io/gorm"
)

type courseRepository struct {
	db *gorm.DB
}

// NewCourseRepository 创建课程仓储实现
func NewCourseRepository(db *gorm.DB) course.Repository {
	return &courseRepository{db: db}
}

func (r *courseRepository) Create(ctx context.Context, input course.CreateCourseInput) (*course.Course, error) {
	m := CourseModel{
		BrandID:     input.BrandID,
		Title:       input.Title,
		Description: input.Description,
		CoverURL:    input.CoverURL,
		Difficulty:  string(input.Difficulty),
		DurationMin: input.DurationMin,
		Type:        string(input.Type),
		Status:      string(course.StatusDraft),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, apperr.ErrInternalF("创建课程失败", err)
	}
	return toCourseDomain(&m), nil
}

func (r *courseRepository) GetByID(ctx context.Context, id int64) (*course.Course, error) {
	var m CourseModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrCourseNotFound, "课程不存在")
		}
		return nil, apperr.ErrInternalF("查询课程失败", err)
	}
	return toCourseDomain(&m), nil
}

func (r *courseRepository) ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*course.Course, int64, error) {
	return r.list(ctx, brandID, offset, limit, "")
}

func (r *courseRepository) ListPublished(ctx context.Context, brandID int64, offset, limit int) ([]*course.Course, int64, error) {
	return r.list(ctx, brandID, offset, limit, string(course.StatusPublished))
}

func (r *courseRepository) list(ctx context.Context, brandID int64, offset, limit int, status string) ([]*course.Course, int64, error) {
	var models []CourseModel
	var total int64

	query := r.db.WithContext(ctx).Model(&CourseModel{}).Where("brand_id = ?", brandID)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询课程列表失败", err)
	}

	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询课程列表失败", err)
	}

	items := make([]*course.Course, len(models))
	for i := range models {
		items[i] = toCourseDomain(&models[i])
	}
	return items, total, nil
}

func (r *courseRepository) Update(ctx context.Context, id int64, input course.UpdateCourseInput) (*course.Course, error) {
	var m CourseModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrCourseNotFound, "课程不存在")
		}
		return nil, apperr.ErrInternalF("查询课程失败", err)
	}

	updates := make(map[string]interface{})
	if input.Title != nil {
		updates["title"] = *input.Title
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.CoverURL != nil {
		updates["cover_url"] = *input.CoverURL
	}
	if input.Difficulty != nil {
		updates["difficulty"] = string(*input.Difficulty)
	}
	if input.DurationMin != nil {
		updates["duration_min"] = *input.DurationMin
	}
	if input.Type != nil {
		updates["type"] = string(*input.Type)
	}

	if len(updates) > 0 {
		if err := r.db.WithContext(ctx).Model(&m).Updates(updates).Error; err != nil {
			return nil, apperr.ErrInternalF("更新课程失败", err)
		}
	}

	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, apperr.ErrInternalF("查询更新后的课程失败", err)
	}
	return toCourseDomain(&m), nil
}

func (r *courseRepository) UpdateStatus(ctx context.Context, id int64, status course.Status) error {
	result := r.db.WithContext(ctx).Model(&CourseModel{}).Where("id = ?", id).Update("status", string(status))
	if result.Error != nil {
		return apperr.ErrInternalF("更新课程状态失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrCourseNotFound, "课程不存在")
	}
	return nil
}

func (r *courseRepository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&CourseModel{}, id)
	if result.Error != nil {
		return apperr.ErrInternalF("删除课程失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrCourseNotFound, "课程不存在")
	}
	return nil
}

func toCourseDomain(m *CourseModel) *course.Course {
	return &course.Course{
		ID:          m.ID,
		BrandID:     m.BrandID,
		Title:       m.Title,
		Description: m.Description,
		CoverURL:    m.CoverURL,
		Difficulty:  course.Difficulty(m.Difficulty),
		DurationMin: m.DurationMin,
		Type:        course.CourseType(m.Type),
		Status:      course.Status(m.Status),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}
