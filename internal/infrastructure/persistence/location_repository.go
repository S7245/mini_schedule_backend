package persistence

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/location"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type locationRepository struct {
	db    *gorm.DB
	guard *commercial.SubscriptionGuard
}

// NewLocationRepository 创建 Location 仓储。
//
// guard 抽到 application/commercial 后注入；Create 时复用三段式
// "锁 active subscription → COUNT → 比 max"（见 SubscriptionGuard.CheckAndCount）。
func NewLocationRepository(db *gorm.DB, guard *commercial.SubscriptionGuard) location.Repository {
	return &locationRepository{db: db, guard: guard}
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
		// 1) SubscriptionGuard 一次性做：SELECT FOR UPDATE active 且未过期 subscription
		//    + COUNT locations（排除软删，含 inactive 防 disable→腾位 hack）+ 比 max。
		//    成功返回 (current, max) 供后续日志；超限返 QUOTA_EXCEEDED + Details。
		if _, _, err := r.guard.CheckAndCount(ctx, tx, input.BrandID, commercial.ResourceLocation); err != nil {
			return err
		}

		// 2) INSERT
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

		// 3) OperationLog 留痕（同事务内）
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
	// Batch 6 T07：data_scope=assigned_locations 收紧。nil = 不限制；空切片 = 拒绝所有。
	if filter.ScopeLocationIDs != nil {
		if len(filter.ScopeLocationIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("id IN ?", filter.ScopeLocationIDs)
		}
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

// CountActiveReferences 统计阻止删除的 active 引用（员工任职 + 门店级角色任职），带 brand_id 隔离。
//
// 软删不触发 FK 行为，会留下指向已删门店的悬空引用，因此删除前先 COUNT：
//   - staff_location_assignments WHERE location_id=? AND brand_id=? AND status='active'
//   - brand_user_role_assignments WHERE location_id=? AND brand_id=? AND status='active'
//
// 两表都硬删重插（只存 active 行），filter active 语义直白；brand_id 防跨租户误计。
func (r *locationRepository) CountActiveReferences(ctx context.Context, brandID, locationID int64) (int64, error) {
	var staffCount int64
	if err := r.db.WithContext(ctx).
		Model(&StaffLocationAssignmentModel{}).
		Where("location_id = ? AND brand_id = ? AND status = ?", locationID, brandID, "active").
		Count(&staffCount).Error; err != nil {
		return 0, apperr.ErrInternalF("统计门店员工任职引用失败", err)
	}

	var roleCount int64
	if err := r.db.WithContext(ctx).
		Model(&BrandUserRoleAssignmentModel{}).
		Where("location_id = ? AND brand_id = ? AND status = ?", locationID, brandID, "active").
		Count(&roleCount).Error; err != nil {
		return 0, apperr.ErrInternalF("统计门店角色任职引用失败", err)
	}

	return staffCount + roleCount, nil
}

// writeLocationOperationLog 在事务内写一条门店生命周期 OperationLog。
// 经 Batch 5 T02 后所有写入都走 audit.Write；actor_type 固定为 brand_user。
func writeLocationOperationLog(tx *gorm.DB, brandID, actorID int64, action string, locationID int64, before, after *LocationModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "location", ID: locationID},
		Before:  before,
		After:   after,
	})
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
