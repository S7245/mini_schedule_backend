package persistence

import (
	"context"
	"errors"
	"strings"

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
		// 1) 锁 brand 的 active subscription
		var sub BrandSubscriptionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("brand_id = ? AND status = ?", input.BrandID, "active").
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
			ae := apperr.NewAppError(apperr.ErrQuotaExceeded, "门店数量已达套餐上限", 409)
			// 让上层能拿到 current/max；约定通过 Error 字符串携带，不再深 wrap
			ae.Err = &quotaDetails{current: current, max: int64(sub.MaxLocations)}
			return ae
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
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toLocationDomain(&created), nil
}

// quotaDetails 是 QUOTA_EXCEEDED 的内部 carrier，供 application 层取 current/max。
type quotaDetails struct {
	current int64
	max     int64
}

func (q *quotaDetails) Error() string { return "quota" }

// QuotaDetailsFromError 解出 quota 详情，没有时返回 false。
func QuotaDetailsFromError(err error) (current, max int64, ok bool) {
	if err == nil {
		return 0, 0, false
	}
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		if q, isQ := ae.Err.(*quotaDetails); isQ && q != nil {
			return q.current, q.max, true
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

func (r *locationRepository) UpdateStatus(ctx context.Context, brandID, id int64, status location.Status) (*location.Location, error) {
	var m LocationModel
	if err := r.db.WithContext(ctx).Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询门店失败", err)
	}

	if m.Status != string(status) {
		if err := r.db.WithContext(ctx).Model(&m).Update("status", string(status)).Error; err != nil {
			return nil, apperr.ErrInternalF("更新门店状态失败", err)
		}
		m.Status = string(status)
	}

	return toLocationDomain(&m), nil
}

func (r *locationRepository) SoftDelete(ctx context.Context, brandID, id int64) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ?", id, brandID).
		Delete(&LocationModel{})
	if res.Error != nil {
		return apperr.ErrInternalF("删除门店失败", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
	}
	return nil
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
