package persistence

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/coursecategory"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type courseCategoryRepository struct {
	db *gorm.DB
}

// NewCourseCategoryRepository 创建课程分类仓储。
func NewCourseCategoryRepository(db *gorm.DB) coursecategory.Repository {
	return &courseCategoryRepository{db: db}
}

func (r *courseCategoryRepository) Create(ctx context.Context, in coursecategory.CreateInput) (*coursecategory.Category, error) {
	var created CourseCategoryModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created = CourseCategoryModel{
			BrandID:           in.BrandID,
			Name:              strings.TrimSpace(in.Name),
			Color:             strings.TrimSpace(in.Color),
			Icon:              strings.TrimSpace(in.Icon),
			SortOrder:         in.SortOrder,
			ShowInMiniProgram: in.ShowInMiniProgram,
			Status:            string(coursecategory.StatusActive),
		}
		if err := tx.Create(&created).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrCategoryNameDuplicated, "同名课程分类已存在", 409)
			}
			return apperr.ErrInternalF("创建课程分类失败", err)
		}
		return writeCategoryLog(tx, in.BrandID, in.ActorID, "category_created", created.ID, nil, &created)
	})
	if err != nil {
		return nil, err
	}
	return toCategoryDomain(&created), nil
}

func (r *courseCategoryRepository) GetByID(ctx context.Context, brandID, id int64) (*coursecategory.Category, error) {
	var m CourseCategoryModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrCategoryNotFound, "课程分类不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询课程分类失败", err)
	}
	return toCategoryDomain(&m), nil
}

func (r *courseCategoryRepository) List(ctx context.Context, filter coursecategory.ListFilter) ([]*coursecategory.Category, error) {
	q := r.db.WithContext(ctx).Model(&CourseCategoryModel{}).Where("brand_id = ?", filter.BrandID)
	if filter.Status == string(coursecategory.StatusActive) || filter.Status == string(coursecategory.StatusInactive) {
		q = q.Where("status = ?", filter.Status)
	}
	var ms []CourseCategoryModel
	if err := q.Order("sort_order ASC, id ASC").Find(&ms).Error; err != nil {
		return nil, apperr.ErrInternalF("查询课程分类列表失败", err)
	}
	items := make([]*coursecategory.Category, len(ms))
	for i := range ms {
		items[i] = toCategoryDomain(&ms[i])
	}
	return items, nil
}

func (r *courseCategoryRepository) Update(ctx context.Context, brandID, actorID, id int64, in coursecategory.UpdateInput) (*coursecategory.Category, error) {
	var after CourseCategoryModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before CourseCategoryModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrCategoryNotFound, "课程分类不存在", 404)
			}
			return apperr.ErrInternalF("查询课程分类失败", err)
		}
		updates := map[string]interface{}{}
		if in.Name != nil {
			updates["name"] = strings.TrimSpace(*in.Name)
		}
		if in.Color != nil {
			updates["color"] = strings.TrimSpace(*in.Color)
		}
		if in.Icon != nil {
			updates["icon"] = strings.TrimSpace(*in.Icon)
		}
		if in.SortOrder != nil {
			updates["sort_order"] = *in.SortOrder
		}
		if in.ShowInMiniProgram != nil {
			updates["show_in_mini_program"] = *in.ShowInMiniProgram
		}
		if in.Status != nil {
			updates["status"] = *in.Status
		}
		if len(updates) > 0 {
			if err := tx.Model(&CourseCategoryModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				if isUniqueViolation(err) {
					return apperr.NewAppError(apperr.ErrCategoryNameDuplicated, "同名课程分类已存在", 409)
				}
				return apperr.ErrInternalF("更新课程分类失败", err)
			}
		}
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&after).Error; err != nil {
			return apperr.ErrInternalF("查询更新后的课程分类失败", err)
		}
		return writeCategoryLog(tx, brandID, actorID, "category_updated", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return toCategoryDomain(&after), nil
}

func (r *courseCategoryRepository) CountActiveByIDs(ctx context.Context, brandID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&CourseCategoryModel{}).
		Where("brand_id = ? AND status = ? AND id IN ?", brandID, "active", ids).
		Count(&count).Error; err != nil {
		return 0, apperr.ErrInternalF("校验课程分类失败", err)
	}
	return count, nil
}

func writeCategoryLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *CourseCategoryModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "course_category", ID: id},
		Before:  before,
		After:   after,
	})
}

func toCategoryDomain(m *CourseCategoryModel) *coursecategory.Category {
	return &coursecategory.Category{
		ID:                m.ID,
		BrandID:           m.BrandID,
		Name:              m.Name,
		Color:             m.Color,
		Icon:              m.Icon,
		SortOrder:         m.SortOrder,
		ShowInMiniProgram: m.ShowInMiniProgram,
		Status:            coursecategory.Status(m.Status),
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}
