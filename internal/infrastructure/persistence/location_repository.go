package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/domain/location"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type locationRepository struct {
	db *gorm.DB
}

// NewLocationRepository 创建 Location 仓储。
func NewLocationRepository(db *gorm.DB) location.Repository {
	return &locationRepository{db: db}
}

// Create 在单一事务内做 quota 校验 + INSERT：
//
//  1. SELECT FOR UPDATE 锁住 brand 的 active subscription（不存在或非 active → SUBSCRIPTION_RESTRICTED）。
//  2. 在锁内 COUNT locations（包含 inactive，排除 deleted） — 避免 disable→腾位 hack。
//  3. count >= subscription.max_locations → QUOTA_EXCEEDED。
//  4. INSERT location；唯一索引冲突 → LOCATION_NAME_DUPLICATED。
//
// 整个流程串行化（同 Batch 3 SELECT FOR UPDATE 经验），避免并发突破上限（E24/E25）。
func (r *locationRepository) Create(ctx context.Context, input location.CreateLocationInput) (*location.Location, error) {
	var created LocationModel

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 锁 brand 的 active 且未过期 subscription（review #2）：
		//    单看 status='active' 不够——expires_at 已过但 cron 没翻状态时仍会被命中。
		//    实际有效窗口：grace_ends_at 存在则用 grace_ends_at；否则用 expires_at。
		now := time.Now().UTC()
		var sub BrandSubscriptionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("brand_id = ? AND status = ? AND (grace_ends_at > ? OR (grace_ends_at IS NULL AND expires_at > ?))",
				input.BrandID, "active", now, now).
			Order("id DESC").
			First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSubscriptionRestricted, "未找到有效订阅", 403)
			}
			return apperr.ErrInternalF("查询订阅失败", err)
		}

		// 2) COUNT active+inactive locations (排除软删)
		var current int64
		if err := tx.Model(&LocationModel{}).
			Where("brand_id = ? AND deleted_at IS NULL", input.BrandID).
			Count(&current).Error; err != nil {
			return apperr.ErrInternalF("统计门店数量失败", err)
		}

		if current >= int64(sub.MaxLocations) {
			// review #6：current/max 走 AppError.Details，由 response.Error 统一序列化。
			return apperr.NewAppError(apperr.ErrQuotaExceeded, "门店数量已达套餐上限", 409).
				WithDetails(map[string]any{
					"current": current,
					"max":     int64(sub.MaxLocations),
				})
		}

		// 3) INSERT
		created = LocationModel{
			BrandID: input.BrandID,
			Name:    strings.TrimSpace(input.Name),
			Address: strings.TrimSpace(input.Address),
			Phone:   strings.TrimSpace(input.Phone),
			Remark:  strings.TrimSpace(input.Remark),
			Status:  string(location.StatusActive),
		}
		if err := tx.Create(&created).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrLocationNameDuplicated, "同名门店已存在", 409)
			}
			return apperr.ErrInternalF("创建门店失败", err)
		}

		// 4) OperationLog 留痕（同事务内）
		return writeLocationOperationLog(tx, input.BrandID, input.ActorID, "location_created", created.ID, nil, &created)
	})
	if err != nil {
		return nil, err
	}

	return toLocationDomain(&created), nil
}

// QuotaDetailsFromError 解出 quota 详情，没有时返回 false。
// 保留供外部（如运营 admin 端）想以编程方式从 AppError.Details 里读 current/max 时使用。
func QuotaDetailsFromError(err error) (current, max int64, ok bool) {
	if err == nil {
		return 0, 0, false
	}
	var ae *apperr.AppError
	if errors.As(err, &ae) && ae != nil && ae.Details != nil {
		c, hasC := ae.Details["current"].(int64)
		m, hasM := ae.Details["max"].(int64)
		if hasC && hasM {
			return c, m, true
		}
	}
	return 0, 0, false
}

func (r *locationRepository) GetByID(ctx context.Context, brandID, id int64) (*location.Location, error) {
	var m LocationModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ?", id, brandID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询门店失败", err)
	}
	return toLocationDomain(&m), nil
}

func (r *locationRepository) List(ctx context.Context, filter location.ListLocationsFilter, offset, limit int) ([]*location.Location, int64, error) {
	q := r.db.WithContext(ctx).Model(&LocationModel{}).Where("brand_id = ?", filter.BrandID)
	if filter.Status == string(location.StatusActive) || filter.Status == string(location.StatusInactive) {
		q = q.Where("status = ?", filter.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询门店列表失败", err)
	}

	var ms []LocationModel
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询门店列表失败", err)
	}

	items := make([]*location.Location, len(ms))
	for i := range ms {
		items[i] = toLocationDomain(&ms[i])
	}
	return items, total, nil
}

func (r *locationRepository) Update(ctx context.Context, brandID, id int64, input location.UpdateLocationInput) (*location.Location, error) {
	var m LocationModel
	if err := r.db.WithContext(ctx).Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询门店失败", err)
	}

	updates := map[string]interface{}{}
	if input.Name != nil {
		updates["name"] = strings.TrimSpace(*input.Name)
	}
	if input.Address != nil {
		updates["address"] = strings.TrimSpace(*input.Address)
	}
	if input.Phone != nil {
		updates["phone"] = strings.TrimSpace(*input.Phone)
	}
	if input.Remark != nil {
		updates["remark"] = strings.TrimSpace(*input.Remark)
	}

	if len(updates) > 0 {
		if err := r.db.WithContext(ctx).Model(&m).Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				return nil, apperr.NewAppError(apperr.ErrLocationNameDuplicated, "同名门店已存在", 409)
			}
			return nil, apperr.ErrInternalF("更新门店失败", err)
		}
	}

	if err := r.db.WithContext(ctx).Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
		return nil, apperr.ErrInternalF("查询更新后的门店失败", err)
	}
	return toLocationDomain(&m), nil
}

func (r *locationRepository) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status location.Status) (*location.Location, error) {
	var m LocationModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
			}
			return apperr.ErrInternalF("查询门店失败", err)
		}
		if m.Status == string(status) {
			// 幂等：状态相同不写日志
			return nil
		}
		before := m
		if err := tx.Model(&m).Update("status", string(status)).Error; err != nil {
			return apperr.ErrInternalF("更新门店状态失败", err)
		}
		m.Status = string(status)
		return writeLocationOperationLog(tx, brandID, actorID, "location_status_changed", id, &before, &m)
	})
	if err != nil {
		return nil, err
	}
	return toLocationDomain(&m), nil
}

func (r *locationRepository) SoftDelete(ctx context.Context, brandID, actorID, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before LocationModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
			}
			return apperr.ErrInternalF("查询门店失败", err)
		}
		res := tx.Where("id = ? AND brand_id = ?", id, brandID).Delete(&LocationModel{})
		if res.Error != nil {
			return apperr.ErrInternalF("删除门店失败", res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
		}
		return writeLocationOperationLog(tx, brandID, actorID, "location_deleted", id, &before, nil)
	})
}

// writeLocationOperationLog 在事务内写一条门店生命周期 OperationLog。
// actor_type 固定为 brand_user；actorID 为 0 时存为 NULL（兼容旧调用 / 系统操作）。
func writeLocationOperationLog(tx *gorm.DB, brandID, actorID int64, action string, locationID int64, before, after *LocationModel) error {
	metadata, err := json.Marshal(map[string]interface{}{
		"before": before,
		"after":  after,
	})
	if err != nil {
		return apperr.ErrInternalF("序列化操作日志失败", err)
	}
	var actorIDPtr *int64
	if actorID > 0 {
		actorIDPtr = &actorID
	}
	bID := brandID
	tID := locationID
	return tx.Create(&OperationLogModel{
		BrandID:    &bID,
		ActorType:  "brand_user",
		ActorID:    actorIDPtr,
		Action:     action,
		TargetType: "location",
		TargetID:   &tID,
		Metadata:   metadata,
	}).Error
}

func toLocationDomain(m *LocationModel) *location.Location {
	return &location.Location{
		ID:        m.ID,
		BrandID:   m.BrandID,
		Name:      m.Name,
		Address:   m.Address,
		Phone:     m.Phone,
		Status:    location.Status(m.Status),
		Remark:    m.Remark,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
