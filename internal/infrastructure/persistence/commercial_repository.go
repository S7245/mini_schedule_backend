package persistence

import (
	"context"
	"encoding/json"
	"time"

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

func createOperationLog(tx *gorm.DB, input operationLogInput) error {
	metadata, err := json.Marshal(map[string]interface{}{
		"before": input.Before,
		"after":  input.After,
	})
	if err != nil {
		return err
	}
	actorID := input.ActorID
	targetID := input.TargetID
	return tx.Create(&OperationLogModel{
		BrandID:    input.BrandID,
		ActorType:  "platform_admin",
		ActorID:    &actorID,
		Action:     input.Action,
		TargetType: input.TargetType,
		TargetID:   &targetID,
		Reason:     input.Reason,
		Metadata:   metadata,
	}).Error
}

func mapBrandSubscriptionError(message string, err error) error {
	if err == gorm.ErrRecordNotFound {
		return apperr.ErrNotFoundF(apperr.ErrNotFound, "品牌订阅不存在")
	}
	return apperr.ErrInternalF(message, err)
}
