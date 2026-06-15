package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/coursetemplate"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type courseTemplateRepository struct {
	db *gorm.DB
}

// NewCourseTemplateRepository 创建课程模板仓储。
func NewCourseTemplateRepository(db *gorm.DB) coursetemplate.Repository {
	return &courseTemplateRepository{db: db}
}

// dedupeInt64 去重并保持顺序，过滤 <=0。
func dedupeInt64(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// validateCategories 校验 ids 全部属本 brand 且 active；空切片直接放行。
func validateCategories(tx *gorm.DB, brandID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&CourseCategoryModel{}).
		Where("brand_id = ? AND status = ? AND id IN ?", brandID, "active", ids).
		Count(&count).Error; err != nil {
		return apperr.ErrInternalF("校验课程分类失败", err)
	}
	if count != int64(len(ids)) {
		return apperr.NewAppError(apperr.ErrCategoryNotFound, "存在无效或未启用的课程分类", 404)
	}
	return nil
}

// resolveLocationIDs 校验/默认化可用门店 id：
//   - 传入非空：校验全部属本 brand active（未软删），不匹配 → INVALID_PARAM。
//   - 传入空：返回当前 brand 全部 active 门店 id（默认全选）。
func resolveLocationIDs(tx *gorm.DB, brandID int64, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		var all []int64
		if err := tx.Model(&LocationModel{}).
			Where("brand_id = ? AND status = ?", brandID, "active").
			Pluck("id", &all).Error; err != nil {
			return nil, apperr.ErrInternalF("查询门店失败", err)
		}
		return all, nil
	}
	var count int64
	if err := tx.Model(&LocationModel{}).
		Where("brand_id = ? AND status = ? AND id IN ?", brandID, "active", ids).
		Count(&count).Error; err != nil {
		return nil, apperr.ErrInternalF("校验门店失败", err)
	}
	if count != int64(len(ids)) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "存在无效或未启用的门店", 400)
	}
	return ids, nil
}

func (r *courseTemplateRepository) Create(ctx context.Context, in coursetemplate.CreateInput) (*coursetemplate.Template, error) {
	categoryIDs := dedupeInt64(in.CategoryIDs)
	var created CourseTemplateModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateCategories(tx, in.BrandID, categoryIDs); err != nil {
			return err
		}
		locationIDs, err := resolveLocationIDs(tx, in.BrandID, dedupeInt64(in.LocationIDs))
		if err != nil {
			return err
		}

		created = CourseTemplateModel{
			BrandID:           in.BrandID,
			Title:             strings.TrimSpace(in.Title),
			Description:       strings.TrimSpace(in.Description),
			CoverURL:          strings.TrimSpace(in.CoverURL),
			LevelLabel:        strings.TrimSpace(in.LevelLabel),
			DurationMin:       in.DurationMin,
			DefaultCapacity:   in.DefaultCapacity,
			ShowInMiniProgram: in.ShowInMiniProgram,
			Status:            string(coursetemplate.StatusDraft),
		}
		if err := tx.Create(&created).Error; err != nil {
			return apperr.ErrInternalF("创建课程模板失败", err)
		}
		if err := replaceCategoryAssignments(tx, in.BrandID, created.ID, categoryIDs); err != nil {
			return err
		}
		if err := replaceLocationAvailability(tx, in.BrandID, created.ID, locationIDs); err != nil {
			return err
		}
		return writeCourseLog(tx, in.BrandID, in.ActorID, "course_created", created.ID, nil, &created)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, created.ID)
}

// replaceCategoryAssignments 硬删重插（镜像 staff_location_assignments 做法）。
func replaceCategoryAssignments(tx *gorm.DB, brandID, courseID int64, categoryIDs []int64) error {
	if err := tx.Where("course_id = ?", courseID).Delete(&CourseCategoryAssignmentModel{}).Error; err != nil {
		return apperr.ErrInternalF("清理课程分类绑定失败", err)
	}
	for _, cid := range categoryIDs {
		row := CourseCategoryAssignmentModel{BrandID: brandID, CourseID: courseID, CategoryID: cid}
		if err := tx.Create(&row).Error; err != nil {
			return apperr.ErrInternalF("写入课程分类绑定失败", err)
		}
	}
	return nil
}

// replaceLocationAvailability 硬删重插，只存 is_available=true 行。
func replaceLocationAvailability(tx *gorm.DB, brandID, courseID int64, locationIDs []int64) error {
	if err := tx.Where("course_id = ?", courseID).Delete(&CourseLocationAvailabilityModel{}).Error; err != nil {
		return apperr.ErrInternalF("清理课程可用门店失败", err)
	}
	for _, lid := range locationIDs {
		row := CourseLocationAvailabilityModel{BrandID: brandID, CourseID: courseID, LocationID: lid, IsAvailable: true}
		if err := tx.Create(&row).Error; err != nil {
			return apperr.ErrInternalF("写入课程可用门店失败", err)
		}
	}
	return nil
}

func (r *courseTemplateRepository) GetByID(ctx context.Context, brandID, id int64) (*coursetemplate.Template, error) {
	var m CourseTemplateModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询课程模板失败", err)
	}
	t := toTemplateDomain(&m)

	cats, err := r.loadCategories(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	t.Categories = cats[id]
	if t.Categories == nil {
		t.Categories = []coursetemplate.CategoryRef{}
	}
	t.CategoryIDs = make([]int64, 0, len(t.Categories))
	for _, c := range t.Categories {
		t.CategoryIDs = append(t.CategoryIDs, c.ID)
	}

	var locIDs []int64
	if err := r.db.WithContext(ctx).Model(&CourseLocationAvailabilityModel{}).
		Where("course_id = ? AND is_available = ?", id, true).
		Order("location_id ASC").Pluck("location_id", &locIDs).Error; err != nil {
		return nil, apperr.ErrInternalF("查询课程可用门店失败", err)
	}
	if locIDs == nil {
		locIDs = []int64{}
	}
	t.AvailableLocationIDs = locIDs
	t.AvailableLocationCount = len(locIDs)
	return t, nil
}

func (r *courseTemplateRepository) List(ctx context.Context, filter coursetemplate.ListFilter, offset, limit int) ([]*coursetemplate.Template, int64, error) {
	q := r.db.WithContext(ctx).Model(&CourseTemplateModel{}).Where("brand_id = ?", filter.BrandID)
	if coursetemplate.IsValidStatus(filter.Status) {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Q != "" {
		q = q.Where("title ILIKE ?", "%"+filter.Q+"%")
	}
	if filter.CategoryID > 0 {
		q = q.Where("id IN (?)", r.db.Model(&CourseCategoryAssignmentModel{}).
			Select("course_id").Where("category_id = ?", filter.CategoryID))
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询课程模板列表失败", err)
	}

	var ms []CourseTemplateModel
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询课程模板列表失败", err)
	}

	ids := make([]int64, len(ms))
	for i := range ms {
		ids[i] = ms[i].ID
	}
	catsByCourse, err := r.loadCategories(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	availByCourse, err := r.loadAvailabilityCounts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*coursetemplate.Template, len(ms))
	for i := range ms {
		t := toTemplateDomain(&ms[i])
		t.Categories = catsByCourse[ms[i].ID]
		if t.Categories == nil {
			t.Categories = []coursetemplate.CategoryRef{}
		}
		t.AvailableLocationCount = availByCourse[ms[i].ID]
		items[i] = t
	}
	return items, total, nil
}

// loadCategories 批量取 course_id → []CategoryRef（避免 N+1）。
func (r *courseTemplateRepository) loadCategories(ctx context.Context, courseIDs []int64) (map[int64][]coursetemplate.CategoryRef, error) {
	out := map[int64][]coursetemplate.CategoryRef{}
	if len(courseIDs) == 0 {
		return out, nil
	}
	type row struct {
		CourseID int64
		ID       int64
		Name     string
		Color    string
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("course_category_assignments a").
		Select("a.course_id, c.id, c.name, c.color").
		Joins("JOIN course_categories c ON c.id = a.category_id").
		Where("a.course_id IN ?", courseIDs).
		Order("c.sort_order ASC, c.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询课程分类绑定失败", err)
	}
	for _, rw := range rows {
		out[rw.CourseID] = append(out[rw.CourseID], coursetemplate.CategoryRef{ID: rw.ID, Name: rw.Name, Color: rw.Color})
	}
	return out, nil
}

// loadAvailabilityCounts 批量取 course_id → available 门店数。
func (r *courseTemplateRepository) loadAvailabilityCounts(ctx context.Context, courseIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(courseIDs) == 0 {
		return out, nil
	}
	type row struct {
		CourseID int64
		Cnt      int
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("course_location_availability").
		Select("course_id, COUNT(*) as cnt").
		Where("course_id IN ? AND is_available = ?", courseIDs, true).
		Group("course_id").
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("统计课程可用门店失败", err)
	}
	for _, rw := range rows {
		out[rw.CourseID] = rw.Cnt
	}
	return out, nil
}

func (r *courseTemplateRepository) Update(ctx context.Context, brandID, actorID, id int64, in coursetemplate.UpdateInput) (*coursetemplate.Template, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before CourseTemplateModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
			}
			return apperr.ErrInternalF("查询课程模板失败", err)
		}

		updates := map[string]interface{}{}
		if in.Title != nil {
			updates["title"] = strings.TrimSpace(*in.Title)
		}
		if in.Description != nil {
			updates["description"] = strings.TrimSpace(*in.Description)
		}
		if in.CoverURL != nil {
			updates["cover_url"] = strings.TrimSpace(*in.CoverURL)
		}
		if in.LevelLabel != nil {
			updates["level_label"] = strings.TrimSpace(*in.LevelLabel)
		}
		if in.DurationMin != nil {
			updates["duration_min"] = *in.DurationMin
		}
		if in.DefaultCapacity != nil {
			updates["default_capacity"] = *in.DefaultCapacity
		}
		if in.ShowInMiniProgram != nil {
			updates["show_in_mini_program"] = *in.ShowInMiniProgram
		}
		if len(updates) > 0 {
			if err := tx.Model(&CourseTemplateModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				return apperr.ErrInternalF("更新课程模板失败", err)
			}
		}

		if in.CategoryIDs != nil {
			categoryIDs := dedupeInt64(*in.CategoryIDs)
			if err := validateCategories(tx, brandID, categoryIDs); err != nil {
				return err
			}
			if err := replaceCategoryAssignments(tx, brandID, id, categoryIDs); err != nil {
				return err
			}
		}
		if in.LocationIDs != nil {
			locationIDs, err := resolveLocationIDs(tx, brandID, dedupeInt64(*in.LocationIDs))
			if err != nil {
				return err
			}
			if err := replaceLocationAvailability(tx, brandID, id, locationIDs); err != nil {
				return err
			}
		}

		var after CourseTemplateModel
		if err := tx.Where("id = ?", id).First(&after).Error; err != nil {
			return apperr.ErrInternalF("查询更新后的课程模板失败", err)
		}
		return writeCourseLog(tx, brandID, actorID, "course_updated", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func (r *courseTemplateRepository) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status coursetemplate.Status) (*coursetemplate.Template, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before CourseTemplateModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
			}
			return apperr.ErrInternalF("查询课程模板失败", err)
		}
		if before.Status == string(status) {
			return nil // 幂等
		}
		updates := map[string]interface{}{"status": string(status)}
		// 首次发布置 published_at；归档/回 draft 不清（留审计痕迹）。
		if status == coursetemplate.StatusPublished && before.PublishedAt == nil {
			now := time.Now().UTC()
			updates["published_at"] = now
		}
		if err := tx.Model(&CourseTemplateModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return apperr.ErrInternalF("更新课程模板状态失败", err)
		}
		var after CourseTemplateModel
		if err := tx.Where("id = ?", id).First(&after).Error; err != nil {
			return apperr.ErrInternalF("查询更新后的课程模板失败", err)
		}
		return writeCourseLog(tx, brandID, actorID, "course_status_changed", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func (r *courseTemplateRepository) SoftDelete(ctx context.Context, brandID, actorID, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before CourseTemplateModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
			}
			return apperr.ErrInternalF("查询课程模板失败", err)
		}
		res := tx.Where("id = ? AND brand_id = ?", id, brandID).Delete(&CourseTemplateModel{})
		if res.Error != nil {
			return apperr.ErrInternalF("删除课程模板失败", res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
		}
		return writeCourseLog(tx, brandID, actorID, "course_deleted", id, &before, nil)
	})
}

func (r *courseTemplateRepository) CountScheduledSessions(ctx context.Context, brandID, courseID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&ClassSessionModel{}).
		Where("brand_id = ? AND course_id = ? AND status IN ?", brandID, courseID, []string{"scheduled", "in_progress"}).
		Count(&count).Error; err != nil {
		return 0, apperr.ErrInternalF("统计课程场次引用失败", err)
	}
	return count, nil
}

func writeCourseLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *CourseTemplateModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "course", ID: id},
		Before:  before,
		After:   after,
	})
}

func toTemplateDomain(m *CourseTemplateModel) *coursetemplate.Template {
	var publishedAt *time.Time
	if m.PublishedAt != nil {
		publishedAt = m.PublishedAt
	}
	return &coursetemplate.Template{
		ID:                m.ID,
		BrandID:           m.BrandID,
		Title:             m.Title,
		Description:       m.Description,
		CoverURL:          m.CoverURL,
		LevelLabel:        m.LevelLabel,
		DurationMin:       m.DurationMin,
		DefaultCapacity:   m.DefaultCapacity,
		ShowInMiniProgram: m.ShowInMiniProgram,
		Status:            coursetemplate.Status(m.Status),
		PublishedAt:       publishedAt,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
		Categories:        []coursetemplate.CategoryRef{},
	}
}
