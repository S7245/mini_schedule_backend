package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type entitlementRepository struct {
	db *gorm.DB
}

// NewEntitlementRepository 创建权益仓储。
func NewEntitlementRepository(db *gorm.DB) entitlement.Repository {
	return &entitlementRepository{db: db}
}

// nullableLimit 把 <=0 归一为 NULL（不限），>0 存值。
func nullableLimit(v int) *int {
	if v <= 0 {
		return nil
	}
	x := v
	return &x
}

// ---- 产品 ----

func (r *entitlementRepository) CreateProduct(ctx context.Context, in entitlement.CreateProductInput) (*entitlement.Product, error) {
	var createdID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locationScope, courseScope := in.LocationScope, in.CourseScope
		locIDs, courseIDs, err := r.resolveProductScope(tx, in.BrandID, locationScope, courseScope, in.LocationIDs, in.CourseIDs)
		if err != nil {
			return err
		}

		var totalCredits *int
		if entitlement.IsCountBased(entitlement.ProductType(in.ProductType)) {
			tc := in.TotalCredits
			totalCredits = &tc
		}

		created := EntitlementProductModel{
			BrandID:                in.BrandID,
			Name:                   strings.TrimSpace(in.Name),
			Description:            strings.TrimSpace(in.Description),
			ProductType:            in.ProductType,
			TotalCredits:           totalCredits,
			ValidityDays:           in.ValidityDays,
			DailyBookingLimit:      nullableLimit(in.DailyBookingLimit),
			WeeklyBookingLimit:     nullableLimit(in.WeeklyBookingLimit),
			MonthlyBookingLimit:    nullableLimit(in.MonthlyBookingLimit),
			ConcurrentBookingLimit: nullableLimit(in.ConcurrentBookingLimit),
			LocationScope:          locationScope,
			CourseScope:            courseScope,
			Status:                 string(entitlement.ProductStatusActive),
		}
		if err := tx.Create(&created).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrEntitlementProductNameDuplicated, "已存在同名启用产品", 409)
			}
			return apperr.ErrInternalF("创建权益产品失败", err)
		}
		createdID = created.ID
		if err := r.replaceProductScope(tx, in.BrandID, created.ID, locationScope, courseScope, locIDs, courseIDs); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetProduct(ctx, in.BrandID, createdID)
}

// resolveProductScope 校验 scope ids：specific 时 ids ⊆ 本 brand（存在性，不含软删）；all 时强制清空 ids。
// 不按 active 过滤——产品创建后门店可能停用 / 课程可能归档，编辑产品（改名等）时仍要能保留既有
// scope 而不被强拒（停用门店/归档课程对 13c 选权益只是不匹配，无害）；新增 scope 时前端只展示
// active/published 选项，故 UI 上不会新绑停用项。门店/课程对称。
func (r *entitlementRepository) resolveProductScope(tx *gorm.DB, brandID int64, locScope, courseScope string, locIDs, courseIDs []int64) ([]int64, []int64, error) {
	outLoc := []int64{}
	if locScope == entitlement.ScopeSpecific {
		outLoc = dedupeInt64(locIDs)
		if len(outLoc) == 0 {
			return nil, nil, apperr.NewAppError(apperr.ErrEntitlementScopeInvalid, "指定门店范围时至少选 1 个门店", 400)
		}
		var cnt int64
		if err := tx.Model(&LocationModel{}).Where("brand_id = ? AND id IN ?", brandID, outLoc).Count(&cnt).Error; err != nil {
			return nil, nil, apperr.ErrInternalF("校验门店失败", err)
		}
		if cnt != int64(len(outLoc)) {
			return nil, nil, apperr.NewAppError(apperr.ErrEntitlementScopeInvalid, "存在无效的门店", 400)
		}
	}
	outCourse := []int64{}
	if courseScope == entitlement.ScopeSpecific {
		outCourse = dedupeInt64(courseIDs)
		if len(outCourse) == 0 {
			return nil, nil, apperr.NewAppError(apperr.ErrEntitlementScopeInvalid, "指定课程范围时至少选 1 个课程", 400)
		}
		var cnt int64
		if err := tx.Model(&CourseTemplateModel{}).Where("brand_id = ? AND id IN ?", brandID, outCourse).Count(&cnt).Error; err != nil {
			return nil, nil, apperr.ErrInternalF("校验课程失败", err)
		}
		if cnt != int64(len(outCourse)) {
			return nil, nil, apperr.NewAppError(apperr.ErrEntitlementScopeInvalid, "存在无效的课程", 400)
		}
	}
	return outLoc, outCourse, nil
}

// replaceProductScope 硬删重插 scope 关联（all 时只删不插）。
func (r *entitlementRepository) replaceProductScope(tx *gorm.DB, brandID, productID int64, locScope, courseScope string, locIDs, courseIDs []int64) error {
	if err := tx.Where("product_id = ?", productID).Delete(&EntitlementProductLocationModel{}).Error; err != nil {
		return apperr.ErrInternalF("清理产品门店范围失败", err)
	}
	if err := tx.Where("product_id = ?", productID).Delete(&EntitlementProductCourseModel{}).Error; err != nil {
		return apperr.ErrInternalF("清理产品课程范围失败", err)
	}
	if locScope == entitlement.ScopeSpecific {
		for _, lid := range locIDs {
			if err := tx.Create(&EntitlementProductLocationModel{BrandID: brandID, ProductID: productID, LocationID: lid}).Error; err != nil {
				return apperr.ErrInternalF("写入产品门店范围失败", err)
			}
		}
	}
	if courseScope == entitlement.ScopeSpecific {
		for _, cid := range courseIDs {
			if err := tx.Create(&EntitlementProductCourseModel{BrandID: brandID, ProductID: productID, CourseID: cid}).Error; err != nil {
				return apperr.ErrInternalF("写入产品课程范围失败", err)
			}
		}
	}
	return nil
}

func (r *entitlementRepository) GetProduct(ctx context.Context, brandID, id int64) (*entitlement.Product, error) {
	var m EntitlementProductModel
	if err := r.db.WithContext(ctx).Where("id = ? AND brand_id = ?", id, brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrEntitlementProductNotFound, "权益产品不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询权益产品失败", err)
	}
	p := toProductDomain(&m)
	var locIDs []int64
	if err := r.db.WithContext(ctx).Model(&EntitlementProductLocationModel{}).
		Where("product_id = ?", id).Order("location_id ASC").Pluck("location_id", &locIDs).Error; err != nil {
		return nil, apperr.ErrInternalF("查询产品门店范围失败", err)
	}
	var courseIDs []int64
	if err := r.db.WithContext(ctx).Model(&EntitlementProductCourseModel{}).
		Where("product_id = ?", id).Order("course_id ASC").Pluck("course_id", &courseIDs).Error; err != nil {
		return nil, apperr.ErrInternalF("查询产品课程范围失败", err)
	}
	if locIDs != nil {
		p.LocationIDs = locIDs
	}
	if courseIDs != nil {
		p.CourseIDs = courseIDs
	}
	counts, err := r.loadIssuedCounts(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	p.IssuedCount = counts[id]
	return p, nil
}

func (r *entitlementRepository) ListProducts(ctx context.Context, filter entitlement.ProductListFilter, offset, limit int) ([]*entitlement.Product, int64, error) {
	q := r.db.WithContext(ctx).Model(&EntitlementProductModel{}).Where("brand_id = ?", filter.BrandID)
	if entitlement.IsValidProductStatus(filter.Status) {
		q = q.Where("status = ?", filter.Status)
	}
	if entitlement.IsValidProductType(filter.ProductType) {
		q = q.Where("product_type = ?", filter.ProductType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询权益产品列表失败", err)
	}
	var ms []EntitlementProductModel
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询权益产品列表失败", err)
	}
	ids := make([]int64, len(ms))
	for i := range ms {
		ids[i] = ms[i].ID
	}
	counts, err := r.loadIssuedCounts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	locMap, courseMap, err := r.loadScopeIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*entitlement.Product, len(ms))
	for i := range ms {
		p := toProductDomain(&ms[i])
		p.IssuedCount = counts[ms[i].ID]
		if v := locMap[ms[i].ID]; v != nil {
			p.LocationIDs = v
		}
		if v := courseMap[ms[i].ID]; v != nil {
			p.CourseIDs = v
		}
		items[i] = p
	}
	return items, total, nil
}

// loadScopeIDs 批量取 product_id → 适用门店/课程 id 列表（specific 产品的 scope）。
// 镜像 loadIssuedCounts：一次 IN 查询避免列表 N+1。all 范围产品无 scope 行，保持空切片。
func (r *entitlementRepository) loadScopeIDs(ctx context.Context, productIDs []int64) (map[int64][]int64, map[int64][]int64, error) {
	locMap := map[int64][]int64{}
	courseMap := map[int64][]int64{}
	if len(productIDs) == 0 {
		return locMap, courseMap, nil
	}
	type locRow struct {
		ProductID  int64
		LocationID int64
	}
	var locRows []locRow
	if err := r.db.WithContext(ctx).
		Table("entitlement_product_locations").
		Select("product_id, location_id").
		Where("product_id IN ?", productIDs).
		Order("product_id ASC, location_id ASC").
		Scan(&locRows).Error; err != nil {
		return nil, nil, apperr.ErrInternalF("查询产品门店范围失败", err)
	}
	for _, rw := range locRows {
		locMap[rw.ProductID] = append(locMap[rw.ProductID], rw.LocationID)
	}
	type courseRow struct {
		ProductID int64
		CourseID  int64
	}
	var courseRows []courseRow
	if err := r.db.WithContext(ctx).
		Table("entitlement_product_courses").
		Select("product_id, course_id").
		Where("product_id IN ?", productIDs).
		Order("product_id ASC, course_id ASC").
		Scan(&courseRows).Error; err != nil {
		return nil, nil, apperr.ErrInternalF("查询产品课程范围失败", err)
	}
	for _, rw := range courseRows {
		courseMap[rw.ProductID] = append(courseMap[rw.ProductID], rw.CourseID)
	}
	return locMap, courseMap, nil
}

// loadIssuedCounts 批量取 product_id → 已发放权益数。
func (r *entitlementRepository) loadIssuedCounts(ctx context.Context, productIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(productIDs) == 0 {
		return out, nil
	}
	type row struct {
		ProductID int64
		Cnt       int
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("learner_entitlements").
		Select("product_id, COUNT(*) as cnt").
		Where("product_id IN ?", productIDs).
		Group("product_id").
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("统计权益发放数失败", err)
	}
	for _, rw := range rows {
		out[rw.ProductID] = rw.Cnt
	}
	return out, nil
}

func (r *entitlementRepository) UpdateProduct(ctx context.Context, brandID, actorID, id int64, in entitlement.UpdateProductInput) (*entitlement.Product, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before EntitlementProductModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrEntitlementProductNotFound, "权益产品不存在", 404)
			}
			return apperr.ErrInternalF("查询权益产品失败", err)
		}

		updates := map[string]interface{}{}
		if in.Name != nil {
			updates["name"] = strings.TrimSpace(*in.Name)
		}
		if in.Description != nil {
			updates["description"] = strings.TrimSpace(*in.Description)
		}
		// total_credits 仅 count-based 产品可改（membership 恒 NULL，type 不可改）。
		if in.TotalCredits != nil && entitlement.IsCountBased(entitlement.ProductType(before.ProductType)) {
			updates["total_credits"] = *in.TotalCredits
		}
		if in.ValidityDays != nil {
			updates["validity_days"] = *in.ValidityDays
		}
		if in.DailyBookingLimit != nil {
			updates["daily_booking_limit"] = nullableLimit(*in.DailyBookingLimit)
		}
		if in.WeeklyBookingLimit != nil {
			updates["weekly_booking_limit"] = nullableLimit(*in.WeeklyBookingLimit)
		}
		if in.MonthlyBookingLimit != nil {
			updates["monthly_booking_limit"] = nullableLimit(*in.MonthlyBookingLimit)
		}
		if in.ConcurrentBookingLimit != nil {
			updates["concurrent_booking_limit"] = nullableLimit(*in.ConcurrentBookingLimit)
		}
		// scope：传了 scope 字段才动；用最终 scope 值（新传或原值）校验+重插关联。
		newLocScope := before.LocationScope
		if in.LocationScope != nil {
			newLocScope = *in.LocationScope
			updates["location_scope"] = newLocScope
		}
		newCourseScope := before.CourseScope
		if in.CourseScope != nil {
			newCourseScope = *in.CourseScope
			updates["course_scope"] = newCourseScope
		}
		if len(updates) > 0 {
			if err := tx.Model(&EntitlementProductModel{}).Where("id = ? AND brand_id = ?", id, brandID).Updates(updates).Error; err != nil {
				if isUniqueViolation(err) {
					return apperr.NewAppError(apperr.ErrEntitlementProductNameDuplicated, "已存在同名启用产品", 409)
				}
				return apperr.ErrInternalF("更新权益产品失败", err)
			}
		}
		// scope 关联：传了 scope 或 ids 之一就重算。
		if in.LocationScope != nil || in.CourseScope != nil || in.LocationIDs != nil || in.CourseIDs != nil {
			locIDsIn := []int64{}
			if in.LocationIDs != nil {
				locIDsIn = *in.LocationIDs
			}
			courseIDsIn := []int64{}
			if in.CourseIDs != nil {
				courseIDsIn = *in.CourseIDs
			}
			locIDs, courseIDs, err := r.resolveProductScope(tx, brandID, newLocScope, newCourseScope, locIDsIn, courseIDsIn)
			if err != nil {
				return err
			}
			if err := r.replaceProductScope(tx, brandID, id, newLocScope, newCourseScope, locIDs, courseIDs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetProduct(ctx, brandID, id)
}

func (r *entitlementRepository) UpdateProductStatus(ctx context.Context, brandID, actorID, id int64, status string) (*entitlement.Product, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before EntitlementProductModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrEntitlementProductNotFound, "权益产品不存在", 404)
			}
			return apperr.ErrInternalF("查询权益产品失败", err)
		}
		if before.Status == status {
			return nil
		}
		if err := tx.Model(&EntitlementProductModel{}).Where("id = ? AND brand_id = ?", id, brandID).Update("status", status).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrEntitlementProductNameDuplicated, "已存在同名启用产品", 409)
			}
			return apperr.ErrInternalF("更新产品状态失败", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetProduct(ctx, brandID, id)
}

// ---- 学员权益 ----

func (r *entitlementRepository) Grant(ctx context.Context, in entitlement.GrantInput) (*entitlement.Entitlement, error) {
	var createdID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 学员属本 brand（未软删）。
		var prof BrandLearnerProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", in.LearnerID, in.BrandID).First(&prof).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
			}
			return apperr.ErrInternalF("查询学员失败", err)
		}
		// 产品属本 brand 且 active。
		var prod EntitlementProductModel
		if err := tx.Where("id = ? AND brand_id = ?", in.ProductID, in.BrandID).First(&prod).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrEntitlementProductNotFound, "权益产品不存在", 404)
			}
			return apperr.ErrInternalF("查询权益产品失败", err)
		}
		if prod.Status != string(entitlement.ProductStatusActive) {
			return apperr.NewAppError(apperr.ErrEntitlementProductInactive, "产品已停用，无法发放", 409)
		}

		starts := time.Now().UTC()
		if in.StartsAt != nil {
			starts = in.StartsAt.UTC()
		}
		expires := starts.AddDate(0, 0, prod.ValidityDays)

		var total, remaining *int
		delta := 0
		if entitlement.IsCountBased(entitlement.ProductType(prod.ProductType)) && prod.TotalCredits != nil {
			t := *prod.TotalCredits
			total, remaining = &t, &t
			delta = t
		}
		actor := in.ActorID
		ent := LearnerEntitlementModel{
			BrandID:               in.BrandID,
			BrandLearnerProfileID: in.LearnerID,
			ProductID:             in.ProductID,
			Status:                string(entitlement.SettleStatus(entitlement.StatusActive, expires, total, remaining, time.Now().UTC())),
			TotalCredits:          total,
			RemainingCredits:      remaining,
			LockedCredits:         0,
			ConsumedCredits:       0,
			StartsAt:              starts,
			ExpiresAt:             expires,
			GrantedBy:             &actor,
			Remark:                strings.TrimSpace(in.Remark),
		}
		if err := tx.Create(&ent).Error; err != nil {
			return apperr.ErrInternalF("发放权益失败", err)
		}
		createdID = ent.ID
		if err := insertEntitlementTransaction(tx, in.BrandID, ent.ID, in.LearnerID, entitlement.ActionGrant, delta, remaining, "开通权益", &actor); err != nil {
			return err
		}
		return writeEntitlementLog(tx, in.BrandID, in.ActorID, "entitlement_granted", ent.ID, nil, &ent)
	})
	if err != nil {
		return nil, err
	}
	return r.GetEntitlement(ctx, in.BrandID, createdID)
}

// settleSweepLearner 把该学员到期/用完的 active 权益落库为 expired/depleted（读触发，无 cron）。
func (r *entitlementRepository) settleSweepLearner(ctx context.Context, brandID, learnerID int64) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE learner_entitlements
		SET status = CASE
			WHEN expires_at <= now() THEN 'expired'
			WHEN total_credits IS NOT NULL AND remaining_credits <= 0 THEN 'depleted'
			ELSE status END,
			updated_at = now()
		WHERE brand_id = ? AND brand_learner_profile_id = ? AND status = 'active'
		  AND (expires_at <= now() OR (total_credits IS NOT NULL AND remaining_credits <= 0))`,
		brandID, learnerID).Error
}

func (r *entitlementRepository) baseEntitlementQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("learner_entitlements e").
		Select("e.*, p.name AS product_name, p.product_type AS product_type").
		Joins("JOIN entitlement_products p ON p.id = e.product_id")
}

// entitlementRow 反范式扫描行。
type entitlementRow struct {
	LearnerEntitlementModel
	ProductName string
	ProductType string
}

func (r *entitlementRepository) ListEntitlementsByLearner(ctx context.Context, brandID, learnerID int64) ([]*entitlement.Entitlement, error) {
	// 学员须属本 brand（与 Grant 一致；防越权读他人/他品牌学员权益，返 404 不泄漏空列表）。
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&BrandLearnerProfileModel{}).
		Where("id = ? AND brand_id = ?", learnerID, brandID).Count(&cnt).Error; err != nil {
		return nil, apperr.ErrInternalF("查询学员失败", err)
	}
	if cnt == 0 {
		return nil, apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
	}
	if err := r.settleSweepLearner(ctx, brandID, learnerID); err != nil {
		return nil, apperr.ErrInternalF("结算权益状态失败", err)
	}
	var rows []entitlementRow
	if err := r.baseEntitlementQuery(ctx).
		Where("e.brand_id = ? AND e.brand_learner_profile_id = ?", brandID, learnerID).
		Order("e.id DESC").Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询学员权益失败", err)
	}
	items := make([]*entitlement.Entitlement, len(rows))
	for i := range rows {
		items[i] = toEntitlementDomain(&rows[i])
	}
	return items, nil
}

func (r *entitlementRepository) GetEntitlement(ctx context.Context, brandID, id int64) (*entitlement.Entitlement, error) {
	var row entitlementRow
	if err := r.baseEntitlementQuery(ctx).Where("e.id = ? AND e.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询权益失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrEntitlementNotFound, "权益不存在", 404)
	}
	return toEntitlementDomain(&row), nil
}

func (r *entitlementRepository) Adjust(ctx context.Context, in entitlement.AdjustInput) (*entitlement.Entitlement, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e LearnerEntitlementModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", in.EntitlementID, in.BrandID).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrEntitlementNotFound, "权益不存在", 404)
			}
			return apperr.ErrInternalF("查询权益失败", err)
		}
		if e.Status == string(entitlement.StatusCancelled) {
			return apperr.NewAppError(apperr.ErrEntitlementNotAdjustable, "已作废权益不可调整", 409)
		}
		if e.TotalCredits == nil || e.RemainingCredits == nil {
			return apperr.NewAppError(apperr.ErrEntitlementInsufficient, "不限次权益无需调整额度", 409)
		}
		newRemaining := *e.RemainingCredits + in.Delta
		if newRemaining < 0 {
			return apperr.NewAppError(apperr.ErrEntitlementInsufficient, "调整后剩余次数不能为负", 409)
		}
		newStatus := entitlement.SettleStatus(entitlement.Status(e.Status), e.ExpiresAt, e.TotalCredits, &newRemaining, time.Now().UTC())
		if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ? AND brand_id = ?", in.EntitlementID, in.BrandID).
			Updates(map[string]interface{}{"remaining_credits": newRemaining, "status": string(newStatus)}).Error; err != nil {
			return apperr.ErrInternalF("调整权益失败", err)
		}
		actor := in.ActorID
		rem := newRemaining
		if err := insertEntitlementTransaction(tx, in.BrandID, e.ID, e.BrandLearnerProfileID, entitlement.ActionManualAdjust, in.Delta, &rem, in.Reason, &actor); err != nil {
			return err
		}
		before := e
		after := e
		after.RemainingCredits = &newRemaining
		after.Status = string(newStatus)
		return writeEntitlementLog(tx, in.BrandID, in.ActorID, "entitlement_adjusted", e.ID, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetEntitlement(ctx, in.BrandID, in.EntitlementID)
}

func (r *entitlementRepository) SetEntitlementStatus(ctx context.Context, brandID, actorID, id int64, status, reason string) (*entitlement.Entitlement, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e LearnerEntitlementModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", id, brandID).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrEntitlementNotFound, "权益不存在", 404)
			}
			return apperr.ErrInternalF("查询权益失败", err)
		}
		if e.Status == string(entitlement.StatusCancelled) {
			return apperr.NewAppError(apperr.ErrEntitlementNotAdjustable, "已作废权益不可再变更状态", 409)
		}
		var target entitlement.Status
		switch status {
		case string(entitlement.StatusFrozen):
			target = entitlement.StatusFrozen
		case string(entitlement.StatusCancelled):
			target = entitlement.StatusCancelled
		case string(entitlement.StatusActive):
			// 恢复：先置 active 再 settle（可能立即又 expired/depleted）。
			target = entitlement.SettleStatus(entitlement.StatusActive, e.ExpiresAt, e.TotalCredits, e.RemainingCredits, time.Now().UTC())
		default:
			return apperr.NewAppError(apperr.ErrInvalidParam, "无效的权益状态", 400)
		}
		if e.Status == string(target) {
			return nil
		}
		if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ? AND brand_id = ?", id, brandID).
			Update("status", string(target)).Error; err != nil {
			return apperr.ErrInternalF("更新权益状态失败", err)
		}
		before := e
		after := e
		after.Status = string(target)
		after.Remark = reason
		return writeEntitlementLog(tx, brandID, actorID, "entitlement_status_changed", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetEntitlement(ctx, brandID, id)
}

func (r *entitlementRepository) ListTransactions(ctx context.Context, brandID, entitlementID int64) ([]*entitlement.Transaction, error) {
	// 先确认权益属本 brand（防越权读流水）。
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&LearnerEntitlementModel{}).
		Where("id = ? AND brand_id = ?", entitlementID, brandID).Count(&cnt).Error; err != nil {
		return nil, apperr.ErrInternalF("查询权益失败", err)
	}
	if cnt == 0 {
		return nil, apperr.NewAppError(apperr.ErrEntitlementNotFound, "权益不存在", 404)
	}
	var ms []EntitlementTransactionModel
	if err := r.db.WithContext(ctx).
		Where("learner_entitlement_id = ? AND brand_id = ?", entitlementID, brandID).
		Order("id DESC").Find(&ms).Error; err != nil {
		return nil, apperr.ErrInternalF("查询权益流水失败", err)
	}
	items := make([]*entitlement.Transaction, len(ms))
	for i := range ms {
		items[i] = toTransactionDomain(&ms[i])
	}
	return items, nil
}

func insertEntitlementTransaction(tx *gorm.DB, brandID, entitlementID, learnerID int64, action entitlement.Action, delta int, balanceAfter *int, note string, operatedBy *int64) error {
	row := EntitlementTransactionModel{
		BrandID:               brandID,
		LearnerEntitlementID:  entitlementID,
		BrandLearnerProfileID: learnerID,
		Action:                string(action),
		DeltaCredits:          delta,
		BalanceAfter:          balanceAfter,
		Note:                  strings.TrimSpace(note),
		OperatedBy:            operatedBy,
	}
	if err := tx.Create(&row).Error; err != nil {
		return apperr.ErrInternalF("写入权益流水失败", err)
	}
	return nil
}

func writeEntitlementLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *LearnerEntitlementModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "learner_entitlement", ID: id},
		Before:  before,
		After:   after,
	})
}

func toProductDomain(m *EntitlementProductModel) *entitlement.Product {
	return &entitlement.Product{
		ID:                     m.ID,
		BrandID:                m.BrandID,
		Name:                   m.Name,
		Description:            m.Description,
		ProductType:            entitlement.ProductType(m.ProductType),
		TotalCredits:           m.TotalCredits,
		ValidityDays:           m.ValidityDays,
		DailyBookingLimit:      m.DailyBookingLimit,
		WeeklyBookingLimit:     m.WeeklyBookingLimit,
		MonthlyBookingLimit:    m.MonthlyBookingLimit,
		ConcurrentBookingLimit: m.ConcurrentBookingLimit,
		LocationScope:          m.LocationScope,
		CourseScope:            m.CourseScope,
		Status:                 entitlement.ProductStatus(m.Status),
		LocationIDs:            []int64{},
		CourseIDs:              []int64{},
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
	}
}

func toEntitlementDomain(r *entitlementRow) *entitlement.Entitlement {
	return &entitlement.Entitlement{
		ID:                    r.ID,
		BrandID:               r.BrandID,
		BrandLearnerProfileID: r.BrandLearnerProfileID,
		ProductID:             r.ProductID,
		ProductName:           r.ProductName,
		ProductType:           entitlement.ProductType(r.ProductType),
		Status:                entitlement.Status(r.Status),
		TotalCredits:          r.TotalCredits,
		RemainingCredits:      r.RemainingCredits,
		LockedCredits:         r.LockedCredits,
		ConsumedCredits:       r.ConsumedCredits,
		StartsAt:              r.StartsAt,
		ExpiresAt:             r.ExpiresAt,
		GrantedBy:             r.GrantedBy,
		Remark:                r.Remark,
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
	}
}

func toTransactionDomain(m *EntitlementTransactionModel) *entitlement.Transaction {
	return &entitlement.Transaction{
		ID:           m.ID,
		Action:       entitlement.Action(m.Action),
		DeltaCredits: m.DeltaCredits,
		BalanceAfter: m.BalanceAfter,
		Note:         m.Note,
		OperatedBy:   m.OperatedBy,
		CreatedAt:    m.CreatedAt,
	}
}
