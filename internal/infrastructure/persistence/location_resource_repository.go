package persistence

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/locationresource"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type locationResourceRepository struct {
	db *gorm.DB
}

// NewLocationResourceRepository 创建资源仓储。
func NewLocationResourceRepository(db *gorm.DB) locationresource.Repository {
	return &locationResourceRepository{db: db}
}

func (r *locationResourceRepository) Create(ctx context.Context, in locationresource.CreateInput) (*locationresource.Resource, error) {
	var createdID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// location 属本 brand 且 active（停用门店不允许新建资源）。
		var loc LocationModel
		if err := tx.Where("id = ? AND brand_id = ?", in.LocationID, in.BrandID).First(&loc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
			}
			return apperr.ErrInternalF("查询门店失败", err)
		}
		if loc.Status != "active" {
			return apperr.NewAppError(apperr.ErrLocationNotFound, "门店已停用，无法新建资源", 409)
		}

		capacity := in.Capacity
		if capacity <= 0 {
			capacity = 1
		}
		created := LocationResourceModel{
			BrandID:    in.BrandID,
			LocationID: in.LocationID,
			Name:       strings.TrimSpace(in.Name),
			Type:       in.Type,
			Capacity:   capacity,
			Status:     string(locationresource.StatusActive),
			Remark:     strings.TrimSpace(in.Remark),
		}
		if err := tx.Create(&created).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrResourceNameDuplicated, "同门店已存在同名资源", 409)
			}
			return apperr.ErrInternalF("创建资源失败", err)
		}
		createdID = created.ID
		return writeResourceOperationLog(tx, in.BrandID, in.ActorID, "location_resource_created", created.ID, nil, &created)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, createdID)
}

// resourceRow 反范式扫描行。
type resourceRow struct {
	LocationResourceModel
	LocationName string
}

func (r *locationResourceRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("location_resources lr").
		Select(`lr.*, l.name AS location_name`).
		Joins("JOIN locations l ON l.id = lr.location_id").
		Where("lr.deleted_at IS NULL")
}

func (r *locationResourceRepository) GetByID(ctx context.Context, brandID, id int64) (*locationresource.Resource, error) {
	var row resourceRow
	if err := r.baseQuery(ctx).Where("lr.id = ? AND lr.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询资源失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
	}
	return toResourceDomain(&row), nil
}

func (r *locationResourceRepository) List(ctx context.Context, filter locationresource.ListFilter, offset, limit int) ([]*locationresource.Resource, int64, error) {
	q := r.baseQuery(ctx).Where("lr.brand_id = ?", filter.BrandID)
	if filter.LocationID > 0 {
		q = q.Where("lr.location_id = ?", filter.LocationID)
	}
	if locationresource.IsValidStatus(filter.Status) {
		q = q.Where("lr.status = ?", filter.Status)
	}
	if filter.ScopeLocationIDs != nil {
		if len(filter.ScopeLocationIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("lr.location_id IN ?", filter.ScopeLocationIDs)
		}
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询资源列表失败", err)
	}

	var rows []resourceRow
	if err := q.Order("lr.id DESC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询资源列表失败", err)
	}
	items := make([]*locationresource.Resource, len(rows))
	for i := range rows {
		items[i] = toResourceDomain(&rows[i])
	}
	return items, total, nil
}

func (r *locationResourceRepository) Update(ctx context.Context, brandID, actorID, id int64, in locationresource.UpdateInput) (*locationresource.Resource, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before LocationResourceModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
			}
			return apperr.ErrInternalF("查询资源失败", err)
		}

		updates := map[string]interface{}{}
		if in.Name != nil {
			updates["name"] = strings.TrimSpace(*in.Name)
		}
		if in.Type != nil {
			updates["type"] = *in.Type
		}
		if in.Capacity != nil {
			updates["capacity"] = *in.Capacity
		}
		if in.Status != nil {
			updates["status"] = *in.Status
		}
		if in.Remark != nil {
			updates["remark"] = strings.TrimSpace(*in.Remark)
		}
		if len(updates) == 0 {
			return nil
		}
		if err := tx.Model(&LocationResourceModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrResourceNameDuplicated, "同门店已存在同名资源", 409)
			}
			return apperr.ErrInternalF("更新资源失败", err)
		}
		var after LocationResourceModel
		if err := tx.Where("id = ?", id).First(&after).Error; err != nil {
			return apperr.ErrInternalF("查询更新后的资源失败", err)
		}
		return writeResourceOperationLog(tx, brandID, actorID, "location_resource_updated", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func (r *locationResourceRepository) Delete(ctx context.Context, brandID, actorID, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before LocationResourceModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
			}
			return apperr.ErrInternalF("查询资源失败", err)
		}

		// 引用保护：被未结束场次或 active 循环排课占用时拒删（软删不触发 FK SET NULL）。
		refs, err := countResourceActiveReferences(tx, brandID, id)
		if err != nil {
			return err
		}
		if refs > 0 {
			return apperr.NewAppError(apperr.ErrResourceInUse, "资源仍被未结束场次或循环排课占用，无法删除", 409)
		}

		res := tx.Where("id = ? AND brand_id = ?", id, brandID).Delete(&LocationResourceModel{})
		if res.Error != nil {
			return apperr.ErrInternalF("删除资源失败", res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
		}
		return writeResourceOperationLog(tx, brandID, actorID, "location_resource_deleted", id, &before, nil)
	})
}

// countResourceActiveReferences 统计阻止删除资源的 active 引用：
//   - class_sessions WHERE location_resource_id=? AND status IN scheduled/in_progress
//   - recurring_schedules WHERE location_resource_id=? AND status='active'（12b 落地前恒 0）
func countResourceActiveReferences(tx *gorm.DB, brandID, resourceID int64) (int64, error) {
	var sessionCount int64
	if err := tx.Model(&ClassSessionModel{}).
		Where("location_resource_id = ? AND brand_id = ? AND status IN ?", resourceID, brandID, []string{"scheduled", "in_progress"}).
		Count(&sessionCount).Error; err != nil {
		return 0, apperr.ErrInternalF("统计资源场次引用失败", err)
	}
	var recurringCount int64
	if err := tx.Table("recurring_schedules").
		Where("location_resource_id = ? AND brand_id = ? AND status = ?", resourceID, brandID, "active").
		Count(&recurringCount).Error; err != nil {
		return 0, apperr.ErrInternalF("统计资源循环排课引用失败", err)
	}
	return sessionCount + recurringCount, nil
}

func writeResourceOperationLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *LocationResourceModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "location_resource", ID: id},
		Before:  before,
		After:   after,
	})
}

func toResourceDomain(r *resourceRow) *locationresource.Resource {
	return &locationresource.Resource{
		ID:           r.ID,
		BrandID:      r.BrandID,
		LocationID:   r.LocationID,
		Name:         r.Name,
		Type:         locationresource.Type(r.Type),
		Capacity:     r.Capacity,
		Status:       locationresource.Status(r.Status),
		Remark:       r.Remark,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		LocationName: r.LocationName,
	}
}
