package persistence

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type commercialRepository struct {
	db *gorm.DB
}

func NewCommercialRepository(db *gorm.DB) commercial.Repository {
	return &commercialRepository{db: db}
}

func (r *commercialRepository) CreateSaaSPlan(ctx context.Context, input commercial.CreateSaaSPlanInput) (*commercial.SaaSPlan, error) {
	m := SaaSPlanModel{
		Name:              input.Name,
		Description:       input.Description,
		MonthlyPrice:      input.MonthlyPrice,
		YearlyPrice:       input.YearlyPrice,
		YearlyDiscountPct: input.YearlyDiscountPct,
		Currency:          input.Currency,
		MaxLocations:      input.MaxLocations,
		MaxStaffSeats:     input.MaxStaffSeats,
		MaxLearners:       input.MaxLearners,
		Status:            string(commercial.SaaSPlanStatusActive),
		SortOrder:         input.SortOrder,
	}
	for _, feature := range input.Features {
		m.Features = append(m.Features, SaaSPlanFeatureModel{
			FeatureCode: feature.FeatureCode,
			Enabled:     feature.Enabled,
		})
	}

	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, apperr.ErrInternalF("创建 SaaS 套餐失败", err)
	}
	return r.GetSaaSPlan(ctx, m.ID)
}

func (r *commercialRepository) GetSaaSPlan(ctx context.Context, id int64) (*commercial.SaaSPlan, error) {
	var m SaaSPlanModel
	if err := r.db.WithContext(ctx).Preload("Features").First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrNotFound, "SaaS 套餐不存在")
		}
		return nil, apperr.ErrInternalF("查询 SaaS 套餐失败", err)
	}
	return toSaaSPlanDomain(&m), nil
}

func (r *commercialRepository) ListSaaSPlans(ctx context.Context, offset, limit int, includeInactive bool) ([]*commercial.SaaSPlan, int64, error) {
	query := r.db.WithContext(ctx).Model(&SaaSPlanModel{})
	if !includeInactive {
		query = query.Where("status = ?", string(commercial.SaaSPlanStatusActive))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询 SaaS 套餐数量失败", err)
	}

	var models []SaaSPlanModel
	if err := query.Preload("Features").Order("sort_order ASC, id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询 SaaS 套餐列表失败", err)
	}
	return toSaaSPlanDomains(models), total, nil
}

func (r *commercialRepository) ListPublicSaaSPlans(ctx context.Context) ([]*commercial.SaaSPlan, error) {
	var models []SaaSPlanModel
	if err := r.db.WithContext(ctx).
		Preload("Features").
		Where("status = ?", string(commercial.SaaSPlanStatusActive)).
		Order("sort_order ASC, id DESC").
		Find(&models).Error; err != nil {
		return nil, apperr.ErrInternalF("查询公开 SaaS 套餐失败", err)
	}
	return toSaaSPlanDomains(models), nil
}

func (r *commercialRepository) UpdateSaaSPlan(ctx context.Context, id int64, input commercial.UpdateSaaSPlanInput) (*commercial.SaaSPlan, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{}
		if input.Name != nil {
			updates["name"] = *input.Name
		}
		if input.Description != nil {
			updates["description"] = *input.Description
		}
		if input.MonthlyPrice != nil {
			updates["monthly_price"] = *input.MonthlyPrice
		}
		if input.YearlyPrice != nil {
			updates["yearly_price"] = *input.YearlyPrice
		}
		if input.YearlyDiscountPct != nil {
			updates["yearly_discount_pct"] = *input.YearlyDiscountPct
		}
		if input.Currency != nil {
			updates["currency"] = *input.Currency
		}
		if input.MaxLocations != nil {
			updates["max_locations"] = *input.MaxLocations
		}
		if input.MaxStaffSeats != nil {
			updates["max_staff_seats"] = *input.MaxStaffSeats
		}
		if input.MaxLearners != nil {
			updates["max_learners"] = *input.MaxLearners
		}
		if input.SortOrder != nil {
			updates["sort_order"] = *input.SortOrder
		}

		if len(updates) > 0 {
			result := tx.Model(&SaaSPlanModel{}).Where("id = ?", id).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}

		if input.Features != nil {
			if err := tx.Where("plan_id = ?", id).Delete(&SaaSPlanFeatureModel{}).Error; err != nil {
				return err
			}
			for _, feature := range *input.Features {
				if err := tx.Create(&SaaSPlanFeatureModel{
					PlanID:      id,
					FeatureCode: feature.FeatureCode,
					Enabled:     feature.Enabled,
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrNotFound, "SaaS 套餐不存在")
		}
		return nil, apperr.ErrInternalF("更新 SaaS 套餐失败", err)
	}
	return r.GetSaaSPlan(ctx, id)
}

func (r *commercialRepository) UpdateSaaSPlanStatus(ctx context.Context, id int64, status commercial.SaaSPlanStatus) error {
	result := r.db.WithContext(ctx).Model(&SaaSPlanModel{}).Where("id = ?", id).Update("status", string(status))
	if result.Error != nil {
		return apperr.ErrInternalF("更新 SaaS 套餐状态失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrNotFound, "SaaS 套餐不存在")
	}
	return nil
}

func (r *commercialRepository) CreatePublicSignupOrder(ctx context.Context, input commercial.CreatePublicSignupOrderRecordInput) (*commercial.PublicSignupOrderResult, error) {
	var result *commercial.PublicSignupOrderResult

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plan SaaSPlanModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Features").
			Where("id = ? AND status = ?", input.PlanID, string(commercial.SaaSPlanStatusActive)).
			First(&plan).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperr.ErrNotFoundF(apperr.ErrNotFound, "SaaS 套餐不存在或已下架")
			}
			return apperr.ErrInternalF("查询 SaaS 套餐失败", err)
		}

		amount, err := amountForBillingCycle(&plan, input.BillingCycle)
		if err != nil {
			return err
		}

		brandModel := BrandModel{
			Name:         input.BrandName,
			LogoURL:      input.LogoURL,
			ContactName:  input.ContactName,
			ContactPhone: input.Phone,
			Status:       "pending",
		}
		if err := tx.Create(&brandModel).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrBrandExists, "手机号已注册品牌", 409)
			}
			return apperr.ErrInternalF("创建品牌失败", err)
		}
		if input.ContactEmail != "" || input.IndustryType != "" {
			updates := map[string]interface{}{}
			if input.ContactEmail != "" {
				updates["contact_email"] = input.ContactEmail
			}
			if input.IndustryType != "" {
				updates["industry_type"] = input.IndustryType
			}
			if err := tx.Model(&BrandModel{}).Where("id = ?", brandModel.ID).Updates(updates).Error; err != nil {
				return apperr.ErrInternalF("更新品牌资料失败", err)
			}
		}

		brandUserModel := BrandUserModel{
			BrandID:      brandModel.ID,
			Phone:        input.Phone,
			PasswordHash: input.PasswordHash,
			Name:         input.ContactName,
			Status:       "active",
		}
		if err := tx.Create(&brandUserModel).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrUserExists, "手机号已注册", 409)
			}
			return apperr.ErrInternalF("创建品牌负责人失败", err)
		}
		if err := tx.Exec("UPDATE brand_users SET is_owner = TRUE WHERE id = ?", brandUserModel.ID).Error; err != nil {
			return apperr.ErrInternalF("标记品牌负责人失败", err)
		}

		// Batch 5: application 注入"分配 brand_owner 角色"等动作。
		if input.OnBrandUserCreated != nil {
			if err := input.OnBrandUserCreated(tx, brandModel.ID, brandUserModel.ID); err != nil {
				return err
			}
		}

		orderModel := SaaSPlanOrderModel{
			BrandID:        brandModel.ID,
			BrandUserID:    &brandUserModel.ID,
			PlanID:         plan.ID,
			Source:         string(commercial.OrderSourcePublicSignupFirstPurchase),
			BillingCycle:   string(input.BillingCycle),
			Amount:         amount,
			Currency:       plan.Currency,
			PaymentChannel: string(input.PaymentChannel),
			Status:         string(commercial.SaaSPlanOrderStatusPendingPayment),
			OutTradeNo:     input.OutTradeNo,
		}
		if err := tx.Create(&orderModel).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrInvalidRequest, "订单号重复，请重试", 409)
			}
			return apperr.ErrInternalF("创建首购订单失败", err)
		}

		result = &commercial.PublicSignupOrderResult{
			BrandID:         brandModel.ID,
			BrandName:       brandModel.Name,
			BrandStatus:     brandModel.Status,
			BrandUserID:     brandUserModel.ID,
			BrandUserPhone:  brandUserModel.Phone,
			BrandUserStatus: brandUserModel.Status,
			Plan:            toSaaSPlanDomain(&plan),
			Order:           toSaaSPlanOrderDomain(&orderModel),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *commercialRepository) ListSaaSPlanOrders(ctx context.Context, offset, limit int, filter commercial.ListSaaSPlanOrdersFilter) ([]*commercial.SaaSPlanOrder, int64, error) {
	query := r.db.WithContext(ctx).Model(&SaaSPlanOrderModel{})
	if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}
	if filter.PaymentChannel != "" {
		query = query.Where("payment_channel = ?", string(filter.PaymentChannel))
	}
	if filter.Source != "" {
		query = query.Where("source = ?", string(filter.Source))
	}
	if filter.BrandID > 0 {
		query = query.Where("brand_id = ?", filter.BrandID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询套餐订单数量失败", err)
	}

	var models []SaaSPlanOrderModel
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询套餐订单列表失败", err)
	}
	items := make([]*commercial.SaaSPlanOrder, len(models))
	for i := range models {
		items[i] = toSaaSPlanOrderDomain(&models[i])
	}
	return items, total, nil
}

func (r *commercialRepository) ListBrandSubscriptions(ctx context.Context, offset, limit int, filter commercial.ListBrandSubscriptionsFilter) ([]*commercial.BrandSubscription, int64, error) {
	query := r.db.WithContext(ctx).Model(&BrandSubscriptionModel{})
	if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}
	if filter.BrandID > 0 {
		query = query.Where("brand_id = ?", filter.BrandID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询品牌订阅数量失败", err)
	}

	var models []BrandSubscriptionModel
	if err := query.Preload("Features").Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询品牌订阅列表失败", err)
	}
	items := make([]*commercial.BrandSubscription, len(models))
	for i := range models {
		items[i] = toBrandSubscriptionDomain(&models[i])
	}
	return items, total, nil
}

func (r *commercialRepository) GetBrandSubscription(ctx context.Context, id int64) (*commercial.BrandSubscription, error) {
	var m BrandSubscriptionModel
	if err := r.db.WithContext(ctx).Preload("Features").First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrNotFound, "品牌订阅不存在")
		}
		return nil, apperr.ErrInternalF("查询品牌订阅失败", err)
	}
	return toBrandSubscriptionDomain(&m), nil
}

func (r *commercialRepository) ManualRenewBrandSubscription(ctx context.Context, id int64, input commercial.ManualRenewBrandSubscriptionInput) (*commercial.BrandSubscription, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandSubscriptionModel
		if err := tx.Preload("Features").First(&before, id).Error; err != nil {
			return err
		}

		now := time.Now()
		base := before.ExpiresAt
		if base.Before(now) {
			base = now
		}
		newExpiresAt := base.AddDate(0, input.ExtendMonths, input.ExtendDays)
		status := before.Status
		if status != string(commercial.BrandSubscriptionStatusFrozen) {
			status = string(commercial.BrandSubscriptionStatusActive)
		}

		updates := map[string]interface{}{
			"expires_at":    newExpiresAt,
			"grace_ends_at": nil,
			"status":        status,
		}
		if err := tx.Model(&BrandSubscriptionModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}

		var after BrandSubscriptionModel
		if err := tx.Preload("Features").First(&after, id).Error; err != nil {
			return err
		}
		return createOperationLog(tx, operationLogInput{
			BrandID:    &after.BrandID,
			ActorID:    input.ActorID,
			Action:     "brand_subscription.manual_renew",
			TargetType: "brand_subscription",
			TargetID:   after.ID,
			Reason:     input.Reason,
			Before:     toBrandSubscriptionDomain(&before),
			After:      toBrandSubscriptionDomain(&after),
		})
	})
	if err != nil {
		return nil, mapBrandSubscriptionError("手动续期品牌订阅失败", err)
	}
	return r.GetBrandSubscription(ctx, id)
}

func (r *commercialRepository) UpdateBrandSubscriptionLimits(ctx context.Context, id int64, input commercial.UpdateBrandSubscriptionLimitsInput) (*commercial.BrandSubscription, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandSubscriptionModel
		if err := tx.Preload("Features").First(&before, id).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{}
		if input.MaxLocations != nil {
			updates["max_locations"] = *input.MaxLocations
		}
		if input.MaxStaffSeats != nil {
			updates["max_staff_seats"] = *input.MaxStaffSeats
		}
		if input.MaxLearners != nil {
			updates["max_learners"] = *input.MaxLearners
		}
		if len(updates) > 0 {
			if err := tx.Model(&BrandSubscriptionModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				return err
			}
		}

		if input.Features != nil {
			if err := tx.Where("subscription_id = ?", id).Delete(&BrandSubscriptionFeatureModel{}).Error; err != nil {
				return err
			}
			for _, feature := range *input.Features {
				if err := tx.Create(&BrandSubscriptionFeatureModel{
					SubscriptionID: id,
					FeatureCode:    feature.FeatureCode,
					Enabled:        feature.Enabled,
				}).Error; err != nil {
					return err
				}
			}
		}

		var after BrandSubscriptionModel
		if err := tx.Preload("Features").First(&after, id).Error; err != nil {
			return err
		}
		return createOperationLog(tx, operationLogInput{
			BrandID:    &after.BrandID,
			ActorID:    input.ActorID,
			Action:     "brand_subscription.update_limits",
			TargetType: "brand_subscription",
			TargetID:   after.ID,
			Reason:     input.Reason,
			Before:     toBrandSubscriptionDomain(&before),
			After:      toBrandSubscriptionDomain(&after),
		})
	})
	if err != nil {
		return nil, mapBrandSubscriptionError("调整品牌订阅额度失败", err)
	}
	return r.GetBrandSubscription(ctx, id)
}

func (r *commercialRepository) UpdateBrandSubscriptionStatus(ctx context.Context, id int64, input commercial.UpdateBrandSubscriptionStatusInput) (*commercial.BrandSubscription, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandSubscriptionModel
		if err := tx.Preload("Features").First(&before, id).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"status": string(input.Status),
		}
		if input.Status == commercial.BrandSubscriptionStatusFrozen {
			updates["frozen_reason"] = input.FrozenReason
		} else {
			updates["frozen_reason"] = ""
		}
		if input.Status == commercial.BrandSubscriptionStatusActive {
			updates["grace_ends_at"] = nil
		}
		if err := tx.Model(&BrandSubscriptionModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}

		var after BrandSubscriptionModel
		if err := tx.Preload("Features").First(&after, id).Error; err != nil {
			return err
		}
		action := "brand_subscription.update_status"
		if input.Status == commercial.BrandSubscriptionStatusFrozen {
			action = "brand_subscription.freeze"
		}
		if input.Status == commercial.BrandSubscriptionStatusActive && before.Status == string(commercial.BrandSubscriptionStatusFrozen) {
			action = "brand_subscription.unfreeze"
		}
		return createOperationLog(tx, operationLogInput{
			BrandID:    &after.BrandID,
			ActorID:    input.ActorID,
			Action:     action,
			TargetType: "brand_subscription",
			TargetID:   after.ID,
			Reason:     input.Reason,
			Before:     toBrandSubscriptionDomain(&before),
			After:      toBrandSubscriptionDomain(&after),
		})
	})
	if err != nil {
		return nil, mapBrandSubscriptionError("更新品牌订阅状态失败", err)
	}
	return r.GetBrandSubscription(ctx, id)
}

func (r *commercialRepository) ListPaymentTransactions(ctx context.Context, offset, limit int, filter commercial.ListPaymentTransactionsFilter) ([]*commercial.PaymentTransaction, int64, error) {
	query := r.db.WithContext(ctx).Model(&PaymentTransactionModel{})
	if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}
	if filter.PaymentChannel != "" {
		query = query.Where("payment_channel = ?", string(filter.PaymentChannel))
	}
	if filter.OrderID > 0 {
		query = query.Where("order_id = ?", filter.OrderID)
	}
	if filter.BrandID > 0 {
		query = query.Where("brand_id = ?", filter.BrandID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询支付流水数量失败", err)
	}

	var models []PaymentTransactionModel
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询支付流水列表失败", err)
	}
	items := make([]*commercial.PaymentTransaction, len(models))
	for i := range models {
		items[i] = toPaymentTransactionDomain(&models[i])
	}
	return items, total, nil
}

func (r *commercialRepository) ListPaymentCallbackLogs(ctx context.Context, offset, limit int, status commercial.PaymentCallbackLogStatus) ([]*commercial.PaymentCallbackLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&PaymentCallbackLogModel{})
	if status != "" {
		query = query.Where("status = ?", string(status))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询支付回调日志数量失败", err)
	}

	var models []PaymentCallbackLogModel
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询支付回调日志列表失败", err)
	}
	items := make([]*commercial.PaymentCallbackLog, len(models))
	for i := range models {
		items[i] = toPaymentCallbackLogDomain(&models[i])
	}
	return items, total, nil
}

func (r *commercialRepository) ListOperationLogs(ctx context.Context, offset, limit int, filter commercial.ListOperationLogsFilter) ([]*commercial.OperationLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&OperationLogModel{})
	if filter.BrandID > 0 {
		query = query.Where("brand_id = ?", filter.BrandID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type = ?", filter.TargetType)
	}
	if filter.TargetID > 0 {
		query = query.Where("target_id = ?", filter.TargetID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询操作日志数量失败", err)
	}

	var models []OperationLogModel
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询操作日志列表失败", err)
	}
	items := make([]*commercial.OperationLog, len(models))
	for i := range models {
		items[i] = toOperationLogDomain(&models[i])
	}
	return items, total, nil
}

func (r *commercialRepository) GetPlatformSummary(ctx context.Context) (*commercial.PlatformSummary, error) {
	var summary commercial.PlatformSummary
	if err := r.db.WithContext(ctx).Model(&BrandModel{}).Count(&summary.BrandTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计品牌数量失败", err)
	}
	if err := r.db.WithContext(ctx).Model(&BrandModel{}).Where("status = ?", "pending").Count(&summary.PendingBrandTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计待开通品牌数量失败", err)
	}
	if err := r.db.WithContext(ctx).Model(&BrandModel{}).Where("status = ?", "active").Count(&summary.ActiveBrandTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计活跃品牌数量失败", err)
	}
	if err := r.db.WithContext(ctx).Model(&BrandSubscriptionModel{}).Where("status = ?", string(commercial.BrandSubscriptionStatusActive)).Count(&summary.ActiveSubscriptionTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计有效订阅数量失败", err)
	}
	now := time.Now()
	in7Days := now.AddDate(0, 0, 7)
	if err := r.db.WithContext(ctx).Model(&BrandSubscriptionModel{}).
		Where("status = ? AND expires_at >= ? AND expires_at < ?", string(commercial.BrandSubscriptionStatusActive), now, in7Days).
		Count(&summary.ExpiringIn7DaysTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计即将到期订阅失败", err)
	}
	if err := r.db.WithContext(ctx).Model(&BrandSubscriptionModel{}).
		Where("status IN ?", []string{string(commercial.BrandSubscriptionStatusRestricted), string(commercial.BrandSubscriptionStatusFrozen)}).
		Count(&summary.RestrictedOrFrozenTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计受限订阅失败", err)
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := r.db.WithContext(ctx).Model(&SaaSPlanOrderModel{}).Where("created_at >= ?", todayStart).Count(&summary.TodayOrderTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计今日订单失败", err)
	}
	var paidAmount string
	if err := r.db.WithContext(ctx).Raw(
		"SELECT COALESCE(SUM(amount), 0)::text FROM saas_plan_orders WHERE status = ? AND paid_at >= ?",
		string(commercial.SaaSPlanOrderStatusPaid),
		todayStart,
	).Scan(&paidAmount).Error; err != nil {
		return nil, apperr.ErrInternalF("统计今日支付金额失败", err)
	}
	summary.TodayPaidAmount = paidAmount
	if err := r.db.WithContext(ctx).Model(&SaaSPlanOrderModel{}).Where("status = ?", string(commercial.SaaSPlanOrderStatusException)).Count(&summary.ExceptionOrderTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计异常订单失败", err)
	}
	if err := r.db.WithContext(ctx).Model(&PaymentCallbackLogModel{}).Where("status = ?", string(commercial.PaymentCallbackLogStatusFailed)).Count(&summary.FailedCallbackTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("统计失败回调数量失败", err)
	}
	return &summary, nil
}

func (r *commercialRepository) CreateWeChatNativePayOrder(ctx context.Context, orderID int64, codeURL, prepayID string, expiresAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&SaaSPlanOrderModel{}).
		Where("id = ? AND status = ?", orderID, "pending_payment").
		Updates(map[string]interface{}{
			"wechat_code_url":    codeURL,
			"wechat_prepay_id":   prepayID,
			"payment_expires_at": expiresAt,
		})
	if result.Error != nil {
		return apperr.ErrInternalF("保存支付下单结果失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrNotFound, "订单不存在或状态不符")
	}
	return nil
}

func (r *commercialRepository) GetSaaSPlanOrderStatus(ctx context.Context, orderID int64) (*commercial.SaaSPlanOrderStatusResult, error) {
	var m SaaSPlanOrderModel
	if err := r.db.WithContext(ctx).Select("status", "paid_at").First(&m, orderID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrNotFound, "订单不存在")
		}
		return nil, apperr.ErrInternalF("查询订单状态失败", err)
	}
	return &commercial.SaaSPlanOrderStatusResult{Status: m.Status, PaidAt: m.PaidAt}, nil
}

func (r *commercialRepository) ExistsPhoneInBrandUsers(ctx context.Context, phone string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&BrandUserModel{}).Where("phone = ?", phone).Count(&count).Error
	if err != nil {
		return false, apperr.ErrInternalF("查询手机号失败", err)
	}
	return count > 0, nil
}

func toSaaSPlanDomains(models []SaaSPlanModel) []*commercial.SaaSPlan {
	items := make([]*commercial.SaaSPlan, len(models))
	for i := range models {
		items[i] = toSaaSPlanDomain(&models[i])
	}
	return items
}

func toSaaSPlanDomain(m *SaaSPlanModel) *commercial.SaaSPlan {
	features := make([]commercial.SaaSPlanFeature, len(m.Features))
	for i := range m.Features {
		features[i] = commercial.SaaSPlanFeature{
			ID:          m.Features[i].ID,
			PlanID:      m.Features[i].PlanID,
			FeatureCode: m.Features[i].FeatureCode,
			Enabled:     m.Features[i].Enabled,
			CreatedAt:   m.Features[i].CreatedAt,
			UpdatedAt:   m.Features[i].UpdatedAt,
		}
	}
	return &commercial.SaaSPlan{
		ID:                m.ID,
		Name:              m.Name,
		Description:       m.Description,
		MonthlyPrice:      m.MonthlyPrice,
		YearlyPrice:       m.YearlyPrice,
		YearlyDiscountPct: m.YearlyDiscountPct,
		Currency:          m.Currency,
		MaxLocations:      m.MaxLocations,
		MaxStaffSeats:     m.MaxStaffSeats,
		MaxLearners:       m.MaxLearners,
		Status:            commercial.SaaSPlanStatus(m.Status),
		SortOrder:         m.SortOrder,
		Features:          features,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func amountForBillingCycle(plan *SaaSPlanModel, cycle commercial.BillingCycle) (string, error) {
	switch cycle {
	case commercial.BillingCycleMonthly:
		return plan.MonthlyPrice, nil
	case commercial.BillingCycleYearly:
		return plan.YearlyPrice, nil
	default:
		return "", apperr.ErrBadRequest("计费周期无效")
	}
}

func toSaaSPlanOrderDomain(m *SaaSPlanOrderModel) *commercial.SaaSPlanOrder {
	return &commercial.SaaSPlanOrder{
		ID:                m.ID,
		BrandID:           m.BrandID,
		BrandUserID:       m.BrandUserID,
		PlanID:            m.PlanID,
		Source:            commercial.OrderSource(m.Source),
		BillingCycle:      commercial.BillingCycle(m.BillingCycle),
		Amount:            m.Amount,
		Currency:          m.Currency,
		PaymentChannel:    commercial.PaymentChannel(m.PaymentChannel),
		Status:            commercial.SaaSPlanOrderStatus(m.Status),
		OutTradeNo:        m.OutTradeNo,
		ThirdPartyTradeNo: m.ThirdPartyTradeNo,
		WeChatCodeURL:     m.WeChatCodeURL,
		WeChatPrepayID:    m.WeChatPrepayID,
		PaymentExpiresAt:  m.PaymentExpiresAt,
		PaidAt:            m.PaidAt,
		ClosedAt:          m.ClosedAt,
		FailureReason:     m.FailureReason,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func toBrandSubscriptionDomain(m *BrandSubscriptionModel) *commercial.BrandSubscription {
	features := make([]commercial.BrandSubscriptionFeature, len(m.Features))
	for i := range m.Features {
		features[i] = commercial.BrandSubscriptionFeature{
			ID:             m.Features[i].ID,
			SubscriptionID: m.Features[i].SubscriptionID,
			FeatureCode:    m.Features[i].FeatureCode,
			Enabled:        m.Features[i].Enabled,
			CreatedAt:      m.Features[i].CreatedAt,
			UpdatedAt:      m.Features[i].UpdatedAt,
		}
	}
	return &commercial.BrandSubscription{
		ID:            m.ID,
		BrandID:       m.BrandID,
		PlanID:        m.PlanID,
		OrderID:       m.OrderID,
		BillingCycle:  commercial.BillingCycle(m.BillingCycle),
		Status:        commercial.BrandSubscriptionStatus(m.Status),
		StartsAt:      m.StartsAt,
		ExpiresAt:     m.ExpiresAt,
		GraceEndsAt:   m.GraceEndsAt,
		MaxLocations:  m.MaxLocations,
		MaxStaffSeats: m.MaxStaffSeats,
		MaxLearners:   m.MaxLearners,
		FrozenReason:  m.FrozenReason,
		Features:      features,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func toPaymentTransactionDomain(m *PaymentTransactionModel) *commercial.PaymentTransaction {
	return &commercial.PaymentTransaction{
		ID:                 m.ID,
		BrandID:            m.BrandID,
		OrderID:            m.OrderID,
		PaymentChannel:     commercial.PaymentChannel(m.PaymentChannel),
		TransactionType:    commercial.PaymentTransactionType(m.TransactionType),
		Status:             commercial.PaymentTransactionStatus(m.Status),
		Amount:             m.Amount,
		Currency:           m.Currency,
		OutTradeNo:         m.OutTradeNo,
		ThirdPartyTradeNo:  m.ThirdPartyTradeNo,
		ProviderRequestID:  m.ProviderRequestID,
		CallbackReceivedAt: m.CallbackReceivedAt,
		PaidAt:             m.PaidAt,
		FailureReason:      m.FailureReason,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

func toPaymentCallbackLogDomain(m *PaymentCallbackLogModel) *commercial.PaymentCallbackLog {
	return &commercial.PaymentCallbackLog{
		ID:                m.ID,
		BrandID:           m.BrandID,
		OrderID:           m.OrderID,
		TransactionID:     m.TransactionID,
		PaymentChannel:    commercial.PaymentChannel(m.PaymentChannel),
		OutTradeNo:        m.OutTradeNo,
		ThirdPartyTradeNo: m.ThirdPartyTradeNo,
		CallbackRequestID: m.CallbackRequestID,
		Status:            commercial.PaymentCallbackLogStatus(m.Status),
		ProcessedAt:       m.ProcessedAt,
		ErrorMessage:      m.ErrorMessage,
		CreatedAt:         m.CreatedAt,
	}
}

func toOperationLogDomain(m *OperationLogModel) *commercial.OperationLog {
	return &commercial.OperationLog{
		ID:         m.ID,
		BrandID:    m.BrandID,
		ActorType:  m.ActorType,
		ActorID:    m.ActorID,
		Action:     m.Action,
		TargetType: m.TargetType,
		TargetID:   m.TargetID,
		Reason:     m.Reason,
		Metadata:   m.Metadata,
		CreatedAt:  m.CreatedAt,
	}
}

type operationLogInput struct {
	BrandID    *int64
	ActorID    int64
	Action     string
	TargetType string
	TargetID   int64
	Reason     string
	Before     interface{}
	After      interface{}
}

// createOperationLog 是历史 platform_admin actor 入口，已切换为统一 audit.Write。
// 行为兼容：actor_type=platform_admin。新代码请直接调 audit.Write。
func createOperationLog(tx *gorm.DB, input operationLogInput) error {
	return audit.Write(tx, audit.Event{
		BrandID: input.BrandID,
		Actor:   audit.Actor{Type: audit.ActorPlatformAdmin, ID: input.ActorID},
		Action:  input.Action,
		Target:  audit.Target{Type: input.TargetType, ID: input.TargetID},
		Reason:  input.Reason,
		Before:  input.Before,
		After:   input.After,
	})
}

func mapBrandSubscriptionError(message string, err error) error {
	if err == gorm.ErrRecordNotFound {
		return apperr.ErrNotFoundF(apperr.ErrNotFound, "品牌订阅不存在")
	}
	return apperr.ErrInternalF(message, err)
}

// WritePaymentCallbackLog 在事务外写入一条 CallbackLog（用于验签失败 / 事务回滚补偿场景）。
func (r *commercialRepository) WritePaymentCallbackLog(ctx context.Context, log commercial.PaymentCallbackLog) error {
	model := PaymentCallbackLogModel{
		BrandID:           log.BrandID,
		OrderID:           log.OrderID,
		TransactionID:     log.TransactionID,
		PaymentChannel:    string(log.PaymentChannel),
		OutTradeNo:        log.OutTradeNo,
		ThirdPartyTradeNo: log.ThirdPartyTradeNo,
		CallbackRequestID: log.CallbackRequestID,
		Status:            string(log.Status),
		ProcessedAt:       log.ProcessedAt,
		ErrorMessage:      log.ErrorMessage,
	}
	if model.PaymentChannel == "" {
		model.PaymentChannel = string(commercial.PaymentChannelWeChat)
	}
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return apperr.ErrInternalF("写入支付回调日志失败", err)
	}
	return nil
}

// ProcessWeChatCallback 在单个事务中推进订单状态机，并按需创建 Subscription / Transaction / OperationLog。
// 详细业务流程见 pds/batches/batch-03-wechat-callback.md。
func (r *commercialRepository) ProcessWeChatCallback(ctx context.Context, input commercial.ProcessWeChatCallbackInput) (*commercial.ProcessWeChatCallbackResult, error) {
	result := &commercial.ProcessWeChatCallbackResult{}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 锁订单
		var order SaaSPlanOrderModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("out_trade_no = ?", input.OutTradeNo).
			First(&order).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// 订单不存在：写一条 ignored 日志（CallbackLog 在事务内即可，因为我们后面 commit 而非 rollback）
				logModel := buildCallbackLogModel(input, nil, nil, nil, commercial.PaymentCallbackLogStatusIgnored, "order not found")
				if err := tx.Create(&logModel).Error; err != nil {
					return apperr.ErrInternalF("写入回调日志失败", err)
				}
				result.Success = false
				result.Message = "order not found"
				result.CallbackLogStatus = commercial.PaymentCallbackLogStatusIgnored
				return nil
			}
			return apperr.ErrInternalF("查询订单失败", err)
		}

		orderIDPtr := order.ID
		brandIDPtr := order.BrandID

		// 2) 幂等：订单已 paid 且 third_party_trade_no 匹配 → 写 processed 日志
		if order.Status == string(commercial.SaaSPlanOrderStatusPaid) {
			// 同一笔成功回调重放：幂等成功
			logModel := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusProcessed, "")
			now := input.ReceivedAt
			logModel.ProcessedAt = &now
			if err := tx.Create(&logModel).Error; err != nil {
				return apperr.ErrInternalF("写入回调日志失败", err)
			}
			result.Success = true
			result.OrderID = order.ID
			result.BrandID = order.BrandID
			result.Message = "idempotent"
			result.CallbackLogStatus = commercial.PaymentCallbackLogStatusProcessed
			return nil
		}

		// 3) 订单已终结但不是 paid：closed / failed / exception / refunded → 忽略
		if isTerminalOrderStatus(order.Status) {
			msg := fmt.Sprintf("order status: %s", order.Status)
			logModel := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusIgnored, msg)
			if err := tx.Create(&logModel).Error; err != nil {
				return apperr.ErrInternalF("写入回调日志失败", err)
			}
			result.Success = false
			result.OrderID = order.ID
			result.BrandID = order.BrandID
			result.Message = "order not in pending_payment: " + order.Status
			result.CallbackLogStatus = commercial.PaymentCallbackLogStatusIgnored
			return nil
		}

		// 4) 校验 trade_state
		switch input.TradeState {
		case commercial.WeChatTradeStateSuccess:
			// 继续往下处理 happy path
		case commercial.WeChatTradeStateClosed:
			now := input.ReceivedAt
			if err := tx.Model(&SaaSPlanOrderModel{}).
				Where("id = ?", order.ID).
				Updates(map[string]interface{}{
					"status":    string(commercial.SaaSPlanOrderStatusClosed),
					"closed_at": now,
				}).Error; err != nil {
				return apperr.ErrInternalF("更新订单为 closed 失败", err)
			}
			logModel := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusProcessed, "trade_state=CLOSED")
			logModel.ProcessedAt = &now
			if err := tx.Create(&logModel).Error; err != nil {
				return apperr.ErrInternalF("写入回调日志失败", err)
			}
			result.Success = false
			result.OrderID = order.ID
			result.BrandID = order.BrandID
			result.Message = "order closed by wechat"
			result.CallbackLogStatus = commercial.PaymentCallbackLogStatusProcessed
			return nil
		case commercial.WeChatTradeStatePayError:
			now := input.ReceivedAt
			if err := tx.Model(&SaaSPlanOrderModel{}).
				Where("id = ?", order.ID).
				Updates(map[string]interface{}{
					"status":         string(commercial.SaaSPlanOrderStatusFailed),
					"failure_reason": "wechat trade_state=PAYERROR",
				}).Error; err != nil {
				return apperr.ErrInternalF("更新订单为 failed 失败", err)
			}
			logModel := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusProcessed, "trade_state=PAYERROR")
			logModel.ProcessedAt = &now
			if err := tx.Create(&logModel).Error; err != nil {
				return apperr.ErrInternalF("写入回调日志失败", err)
			}
			result.Success = false
			result.OrderID = order.ID
			result.BrandID = order.BrandID
			result.Message = "wechat trade_state=PAYERROR"
			result.CallbackLogStatus = commercial.PaymentCallbackLogStatusProcessed
			return nil
		default:
			msg := fmt.Sprintf("non-success trade_state: %s", input.TradeState)
			logModel := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusIgnored, msg)
			if err := tx.Create(&logModel).Error; err != nil {
				return apperr.ErrInternalF("写入回调日志失败", err)
			}
			result.Success = false
			result.OrderID = order.ID
			result.BrandID = order.BrandID
			result.Message = msg
			result.CallbackLogStatus = commercial.PaymentCallbackLogStatusIgnored
			return nil
		}

		// 5) 金额 / 币种校验
		orderAmountFen, parseErr := parseAmountToFen(order.Amount)
		if parseErr != nil {
			return apperr.ErrInternalF("解析订单金额失败", parseErr)
		}
		expectedCurrency := strings.ToUpper(order.Currency)
		if expectedCurrency == "" {
			expectedCurrency = "CNY"
		}
		actualCurrency := strings.ToUpper(input.Currency)
		if actualCurrency == "" {
			actualCurrency = "CNY"
		}

		if input.Amount != orderAmountFen || actualCurrency != expectedCurrency {
			errMsg := fmt.Sprintf(
				"amount mismatch: expected %d %s, got %d %s",
				orderAmountFen, expectedCurrency, input.Amount, actualCurrency,
			)
			if err := tx.Model(&SaaSPlanOrderModel{}).
				Where("id = ?", order.ID).
				Updates(map[string]interface{}{
					"status":         string(commercial.SaaSPlanOrderStatusException),
					"failure_reason": errMsg,
				}).Error; err != nil {
				return apperr.ErrInternalF("更新订单为 exception 失败", err)
			}
			logModel := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusFailed, errMsg)
			if err := tx.Create(&logModel).Error; err != nil {
				return apperr.ErrInternalF("写入回调日志失败", err)
			}
			// OperationLog: payment_amount_mismatch（通过 audit pkg）
			if err := audit.Write(tx, audit.Event{
				BrandID: &order.BrandID,
				Actor:   audit.Actor{Type: audit.ActorSystem},
				Action:  "payment_amount_mismatch",
				Target:  audit.Target{Type: "saas_plan_order", ID: order.ID},
				Reason:  errMsg,
				After: map[string]interface{}{
					"expected_amount_fen":  orderAmountFen,
					"expected_currency":    expectedCurrency,
					"actual_amount_fen":    input.Amount,
					"actual_currency":      actualCurrency,
					"out_trade_no":         order.OutTradeNo,
					"third_party_trade_no": input.ThirdPartyTradeNo,
				},
			}); err != nil {
				return apperr.ErrInternalF("写入操作日志失败", err)
			}
			result.Success = false
			result.OrderID = order.ID
			result.BrandID = order.BrandID
			result.Message = errMsg
			result.CallbackLogStatus = commercial.PaymentCallbackLogStatusFailed
			return nil
		}

		// 6) Happy path
		// 6.1 取 plan 信息以便计算订阅周期 / 创建 BrandSubscription
		var plan SaaSPlanModel
		if err := tx.Preload("Features").First(&plan, order.PlanID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperr.ErrNotFoundF(apperr.ErrNotFound, "套餐不存在")
			}
			return apperr.ErrInternalF("查询套餐失败", err)
		}

		now := input.ReceivedAt
		successTime := now
		if input.SuccessTime != nil {
			successTime = *input.SuccessTime
		}

		// 6.2 先写 CallbackLog(received)，拿 id
		callbackLog := buildCallbackLogModel(input, &orderIDPtr, &brandIDPtr, nil, commercial.PaymentCallbackLogStatusReceived, "")
		if err := tx.Create(&callbackLog).Error; err != nil {
			return apperr.ErrInternalF("写入回调日志失败", err)
		}

		// 6.3 写 PaymentTransaction(succeeded)
		amountStr := fenToAmountString(input.Amount)
		txModel := PaymentTransactionModel{
			BrandID:            &order.BrandID,
			OrderID:            &order.ID,
			PaymentChannel:     string(input.PaymentChannel),
			TransactionType:    string(commercial.PaymentTransactionTypePayment),
			Status:             string(commercial.PaymentTransactionStatusSucceeded),
			Amount:             amountStr,
			Currency:           actualCurrency,
			OutTradeNo:         input.OutTradeNo,
			ThirdPartyTradeNo:  input.ThirdPartyTradeNo,
			ProviderRequestID:  input.CallbackRequestID,
			CallbackReceivedAt: &now,
			PaidAt:             &successTime,
		}
		if txModel.PaymentChannel == "" {
			txModel.PaymentChannel = string(commercial.PaymentChannelWeChat)
		}
		if err := tx.Create(&txModel).Error; err != nil {
			return apperr.ErrInternalF("写入支付流水失败", err)
		}

		// 6.4 更新订单为 paid
		orderUpdates := map[string]interface{}{
			"status":               string(commercial.SaaSPlanOrderStatusPaid),
			"paid_at":              successTime,
			"third_party_trade_no": input.ThirdPartyTradeNo,
		}
		if err := tx.Model(&SaaSPlanOrderModel{}).
			Where("id = ?", order.ID).
			Updates(orderUpdates).Error; err != nil {
			return apperr.ErrInternalF("更新订单为 paid 失败", err)
		}

		// 6.5 创建 BrandSubscription（按 billing_cycle 计算到期时间）
		startsAt := now
		expiresAt, cycleErr := computeSubscriptionExpiry(startsAt, commercial.BillingCycle(order.BillingCycle))
		if cycleErr != nil {
			return cycleErr
		}
		subModel := BrandSubscriptionModel{
			BrandID:       order.BrandID,
			PlanID:        plan.ID,
			OrderID:       &order.ID,
			BillingCycle:  order.BillingCycle,
			Status:        string(commercial.BrandSubscriptionStatusActive),
			StartsAt:      startsAt,
			ExpiresAt:     expiresAt,
			MaxLocations:  plan.MaxLocations,
			MaxStaffSeats: plan.MaxStaffSeats,
			MaxLearners:   plan.MaxLearners,
		}
		if err := tx.Create(&subModel).Error; err != nil {
			return apperr.ErrInternalF("创建品牌订阅失败", err)
		}
		// 把 plan features 复制为 subscription features
		for _, f := range plan.Features {
			if err := tx.Create(&BrandSubscriptionFeatureModel{
				SubscriptionID: subModel.ID,
				FeatureCode:    f.FeatureCode,
				Enabled:        f.Enabled,
			}).Error; err != nil {
				return apperr.ErrInternalF("写入订阅功能快照失败", err)
			}
		}

		// 6.6 激活 Brand（仅当当前为 pending）
		if err := tx.Model(&BrandModel{}).
			Where("id = ? AND status = ?", order.BrandID, "pending").
			Update("status", "active").Error; err != nil {
			return apperr.ErrInternalF("激活品牌失败", err)
		}

		// 6.7 更新 CallbackLog → processed，回填 transaction_id
		txIDPtr := txModel.ID
		if err := tx.Model(&PaymentCallbackLogModel{}).
			Where("id = ?", callbackLog.ID).
			Updates(map[string]interface{}{
				"status":         string(commercial.PaymentCallbackLogStatusProcessed),
				"processed_at":   now,
				"transaction_id": &txIDPtr,
			}).Error; err != nil {
			return apperr.ErrInternalF("更新回调日志失败", err)
		}

		// 6.8 写 OperationLog（通过 audit pkg）
		if err := audit.Write(tx, audit.Event{
			BrandID: &order.BrandID,
			Actor:   audit.Actor{Type: audit.ActorSystem},
			Action:  "payment_callback_success",
			Target:  audit.Target{Type: "saas_plan_order", ID: order.ID},
			After: map[string]interface{}{
				"order_id":             order.ID,
				"transaction_id":       txModel.ID,
				"subscription_id":      subModel.ID,
				"amount_fen":           input.Amount,
				"currency":             actualCurrency,
				"third_party_trade_no": input.ThirdPartyTradeNo,
			},
		}); err != nil {
			return apperr.ErrInternalF("写入操作日志失败", err)
		}

		result.Success = true
		result.OrderID = order.ID
		result.BrandID = order.BrandID
		result.SubscriptionID = subModel.ID
		result.Message = "ok"
		result.CallbackLogStatus = commercial.PaymentCallbackLogStatusProcessed
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func buildCallbackLogModel(
	input commercial.ProcessWeChatCallbackInput,
	orderID *int64,
	brandID *int64,
	transactionID *int64,
	status commercial.PaymentCallbackLogStatus,
	errMsg string,
) PaymentCallbackLogModel {
	channel := string(input.PaymentChannel)
	if channel == "" {
		channel = string(commercial.PaymentChannelWeChat)
	}
	return PaymentCallbackLogModel{
		BrandID:           brandID,
		OrderID:           orderID,
		TransactionID:     transactionID,
		PaymentChannel:    channel,
		OutTradeNo:        input.OutTradeNo,
		ThirdPartyTradeNo: input.ThirdPartyTradeNo,
		CallbackRequestID: input.CallbackRequestID,
		Status:            string(status),
		ErrorMessage:      errMsg,
		RawBody:           input.RawPayload,
	}
}

func isTerminalOrderStatus(status string) bool {
	switch commercial.SaaSPlanOrderStatus(status) {
	case commercial.SaaSPlanOrderStatusClosed,
		commercial.SaaSPlanOrderStatusFailed,
		commercial.SaaSPlanOrderStatusException,
		commercial.SaaSPlanOrderStatusRefunding,
		commercial.SaaSPlanOrderStatusRefunded:
		return true
	}
	return false
}

// parseAmountToFen 把 numeric(12,2) 字符串（如 "99.00"、"99"、"99.5"）转成 int64 分。
func parseAmountToFen(amount string) (int64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return 0, fmt.Errorf("empty amount")
	}
	neg := false
	if strings.HasPrefix(amount, "-") {
		neg = true
		amount = amount[1:]
	}
	parts := strings.SplitN(amount, ".", 2)
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer part: %w", err)
	}
	var cents int64
	if len(parts) == 2 {
		frac := parts[1]
		switch {
		case len(frac) == 0:
			cents = 0
		case len(frac) == 1:
			d, err := strconv.ParseInt(frac, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid fractional part: %w", err)
			}
			cents = d * 10
		default:
			// 取前 2 位，多余截断
			d, err := strconv.ParseInt(frac[:2], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid fractional part: %w", err)
			}
			cents = d
		}
	}
	total := yuan*100 + cents
	if neg {
		total = -total
	}
	return total, nil
}

// fenToAmountString 把 int64 分转成 "x.yz" 字符串，便于写入 numeric(12,2)。
func fenToAmountString(fen int64) string {
	neg := false
	if fen < 0 {
		neg = true
		fen = -fen
	}
	yuan := fen / 100
	cents := fen % 100
	s := fmt.Sprintf("%d.%02d", yuan, cents)
	if neg {
		s = "-" + s
	}
	return s
}

// computeSubscriptionExpiry 根据 billing_cycle 推算订阅到期时间。
func computeSubscriptionExpiry(startsAt time.Time, cycle commercial.BillingCycle) (time.Time, error) {
	switch cycle {
	case commercial.BillingCycleMonthly:
		return startsAt.AddDate(0, 1, 0), nil
	case commercial.BillingCycleYearly:
		return startsAt.AddDate(1, 0, 0), nil
	default:
		return time.Time{}, apperr.ErrInternal("未知的计费周期: " + string(cycle))
	}
}
