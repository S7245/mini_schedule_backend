package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
	"github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	"github.com/zkw/mini-schedule/backend/internal/domain/learner"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type bookingRepository struct {
	db *gorm.DB
}

// NewBookingRepository 创建预约仓储。
func NewBookingRepository(db *gorm.DB) booking.Repository {
	return &bookingRepository{db: db}
}

// bookingConflictError 把 bookings 唯一冲突（partial unique active）分流成 BOOKING_DUPLICATE。
func bookingConflictError(err error) error {
	name, ok := uniqueConstraint(err)
	if !ok {
		return nil
	}
	switch {
	case strings.Contains(name, "session_learner"):
		return apperr.NewAppError(apperr.ErrBookingDuplicate, "该学员对该场次已有预约", 409)
	case strings.Contains(name, "holds_booking"):
		return apperr.NewAppError(apperr.ErrBookingDuplicate, "该预约已绑定权益", 409)
	default:
		return apperr.NewAppError(apperr.ErrBookingDuplicate, "预约已存在", 409)
	}
}

func int64InSlice(v int64, s []int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// minLimitPtr 叠加取最严：返回两个限额中较小的非 nil 值；都 nil 返 nil（不限）。
func minLimitPtr(a, b *int) *int {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case *a <= *b:
		return a
	default:
		return b
	}
}

// ---- 反范式查询 ----

type bookingRow struct {
	BookingModel
	SessionStartsAt time.Time `gorm:"column:session_starts_at"`
	SessionEndsAt   time.Time `gorm:"column:session_ends_at"`
	SessionStatus   string    `gorm:"column:session_status"`
	LocationID      int64     `gorm:"column:location_id"`
	CourseTitle     string    `gorm:"column:course_title"`
	LocationName    string    `gorm:"column:location_name"`
	LearnerName     string    `gorm:"column:learner_name"`
	LearnerPhone    *string   `gorm:"column:learner_phone"`
	HoldID          *int64    `gorm:"column:hold_id"`
	HoldEntitlement *int64    `gorm:"column:hold_entitlement_id"`
	HoldStatus      *string   `gorm:"column:hold_status"`
	HoldCredits     *int      `gorm:"column:hold_credits"`
	HoldProductName *string   `gorm:"column:hold_product_name"`
}

func (r *bookingRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("bookings b").
		Select(`b.*,
			cs.starts_at AS session_starts_at, cs.ends_at AS session_ends_at, cs.status AS session_status, cs.location_id,
			c.title AS course_title, l.name AS location_name,
			COALESCE(NULLIF(p.nickname, ''), li.nickname) AS learner_name, li.phone AS learner_phone,
			h.id AS hold_id, h.learner_entitlement_id AS hold_entitlement_id, h.status AS hold_status, h.credits AS hold_credits,
			ep.name AS hold_product_name`).
		Joins("JOIN class_sessions cs ON cs.id = b.class_session_id").
		Joins("JOIN courses c ON c.id = cs.course_id").
		Joins("JOIN locations l ON l.id = cs.location_id").
		Joins("JOIN brand_learner_profiles p ON p.id = b.brand_learner_profile_id").
		Joins("JOIN learner_identities li ON li.id = p.learner_identity_id").
		Joins("LEFT JOIN entitlement_holds h ON h.booking_id = b.id AND h.status <> 'released'").
		Joins("LEFT JOIN learner_entitlements le ON le.id = h.learner_entitlement_id").
		Joins("LEFT JOIN entitlement_products ep ON ep.id = le.product_id")
}

func toBookingDomain(r *bookingRow) *booking.Booking {
	b := &booking.Booking{
		ID:                     r.ID,
		BrandID:                r.BrandID,
		ClassSessionID:         r.ClassSessionID,
		BrandLearnerProfileID:  r.BrandLearnerProfileID,
		Source:                 booking.Source(r.Source),
		Status:                 booking.Status(r.Status),
		BookedAt:               r.BookedAt,
		CancelledAt:            r.CancelledAt,
		CancelledBy:            r.CancelledBy,
		CancelReason:           r.CancelReason,
		AssistedBy:             r.AssistedBy,
		RequiresEntitlementFix: r.RequiresEntitlementFix,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		SessionStartsAt:        r.SessionStartsAt,
		SessionEndsAt:          r.SessionEndsAt,
		SessionStatus:          r.SessionStatus,
		CourseTitle:            r.CourseTitle,
		LocationID:             r.LocationID,
		LocationName:           r.LocationName,
		LearnerName:            r.LearnerName,
	}
	if r.CancelSource != nil {
		b.CancelSource = *r.CancelSource
	}
	if r.NoEntitlementReason != nil {
		b.NoEntitlementReason = *r.NoEntitlementReason
	}
	if r.LearnerPhone != nil {
		b.LearnerPhone = *r.LearnerPhone
	}
	if r.HoldID != nil && r.HoldEntitlement != nil {
		h := &booking.Hold{ID: *r.HoldID, LearnerEntitlementID: *r.HoldEntitlement}
		if r.HoldStatus != nil {
			h.Status = booking.HoldStatus(*r.HoldStatus)
		}
		if r.HoldCredits != nil {
			h.Credits = *r.HoldCredits
		}
		if r.HoldProductName != nil {
			h.ProductName = *r.HoldProductName
		}
		b.Hold = h
	}
	return b
}

func (r *bookingRepository) GetByID(ctx context.Context, brandID, id int64) (*booking.Booking, error) {
	var row bookingRow
	if err := r.baseQuery(ctx).Where("b.id = ? AND b.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询预约失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
	}
	return toBookingDomain(&row), nil
}

func (r *bookingRepository) List(ctx context.Context, f booking.ListFilter, offset, limit int) ([]*booking.Booking, int64, error) {
	q := r.baseQuery(ctx).Where("b.brand_id = ?", f.BrandID)
	if f.ClassSessionID > 0 {
		q = q.Where("b.class_session_id = ?", f.ClassSessionID)
	}
	if f.LocationID > 0 {
		q = q.Where("cs.location_id = ?", f.LocationID)
	}
	if f.BrandLearnerProfileID > 0 {
		q = q.Where("b.brand_learner_profile_id = ?", f.BrandLearnerProfileID)
	}
	if f.Status != "" {
		q = q.Where("b.status = ?", f.Status)
	}
	if f.RequiresEntitlementFix != nil {
		q = q.Where("b.requires_entitlement_fix = ?", *f.RequiresEntitlementFix)
	}
	if f.ScopeLocationIDs != nil {
		if len(f.ScopeLocationIDs) == 0 {
			return []*booking.Booking{}, 0, nil
		}
		q = q.Where("cs.location_id IN ?", f.ScopeLocationIDs)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("统计预约失败", err)
	}
	var rows []bookingRow
	if err := q.Order("b.booked_at DESC, b.id DESC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询预约列表失败", err)
	}
	items := make([]*booking.Booking, len(rows))
	for i := range rows {
		items[i] = toBookingDomain(&rows[i])
	}
	return items, total, nil
}

// ---- 预约规则解析 ----

func policyFromModel(m *BrandBookingPolicyModel) booking.Policy {
	return booking.Policy{
		BookAheadMinMinutes:       m.BookAheadMinMinutes,
		BookAheadMaxMinutes:       m.BookAheadMaxMinutes,
		CancelDeadlineMinutes:     m.CancelDeadlineMinutes,
		ReleaseOnCancel:           m.ReleaseOnCancel,
		NoShowConsumesEntitlement: m.NoShowConsumesEntitlement,
		DailyBookingLimit:         m.DailyBookingLimit,
		WeeklyBookingLimit:        m.WeeklyBookingLimit,
		ConcurrentBookingLimit:    m.ConcurrentBookingLimit,
		AllowWaitlist:             m.AllowWaitlist,
		WaitlistLimit:             m.WaitlistLimit,
	}
}

// resolveEffectivePolicy base = location 行(存在) 否则 brand-default 行 否则 DefaultPolicy；叠场次 override。
func (r *bookingRepository) resolveEffectivePolicy(tx *gorm.DB, brandID, locationID, sessionID int64) (booking.EffectivePolicy, error) {
	base := booking.DefaultPolicy()
	var loc BrandBookingPolicyModel
	err := tx.Where("brand_id = ? AND location_id = ?", brandID, locationID).First(&loc).Error
	if err == nil {
		base = policyFromModel(&loc)
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		var def BrandBookingPolicyModel
		derr := tx.Where("brand_id = ? AND location_id IS NULL", brandID).First(&def).Error
		if derr == nil {
			base = policyFromModel(&def)
		} else if !errors.Is(derr, gorm.ErrRecordNotFound) {
			return booking.EffectivePolicy{}, apperr.ErrInternalF("查询品牌预约策略失败", derr)
		}
	} else {
		return booking.EffectivePolicy{}, apperr.ErrInternalF("查询门店预约策略失败", err)
	}

	var ov *booking.PolicyOverride
	var om ClassSessionPolicyOverrideModel
	oerr := tx.Where("brand_id = ? AND class_session_id = ?", brandID, sessionID).First(&om).Error
	if oerr == nil {
		ov = &booking.PolicyOverride{
			AllowCancel:               om.AllowCancel,
			CancelDeadlineMinutes:     om.CancelDeadlineMinutes,
			ReleaseOnCancel:           om.ReleaseOnCancel,
			NoShowConsumesEntitlement: om.NoShowConsumesEntitlement,
			AllowWaitlist:             om.AllowWaitlist,
			WaitlistLimit:             om.WaitlistLimit,
		}
	} else if !errors.Is(oerr, gorm.ErrRecordNotFound) {
		return booking.EffectivePolicy{}, apperr.ErrInternalF("查询场次覆盖策略失败", oerr)
	}
	return booking.ResolveEffectivePolicy(base, ov), nil
}

// ---- 频次计数 ----

func dayBounds(t time.Time) (time.Time, time.Time) {
	s := t.UTC().Truncate(24 * time.Hour)
	return s, s.Add(24 * time.Hour)
}

func weekBounds(t time.Time) (time.Time, time.Time) {
	day := t.UTC().Truncate(24 * time.Hour)
	// 周一为一周起点。
	offset := (int(day.Weekday()) + 6) % 7
	start := day.AddDate(0, 0, -offset)
	return start, start.AddDate(0, 0, 7)
}

func monthBounds(t time.Time) (time.Time, time.Time) {
	u := t.UTC()
	start := time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

type freqCounts struct{ day, week, month, concurrent int }

// countBookingsInRange 统计该学员「非取消」预约中 session.starts_at 落在 [from,to) 的数量。
func (r *bookingRepository) countBookingsInRange(tx *gorm.DB, brandID, learnerID int64, from, to time.Time) (int, error) {
	var n int64
	err := tx.Table("bookings b").
		Joins("JOIN class_sessions cs ON cs.id = b.class_session_id").
		Where("b.brand_id = ? AND b.brand_learner_profile_id = ? AND b.status <> 'cancelled'", brandID, learnerID).
		Where("cs.starts_at >= ? AND cs.starts_at < ?", from, to).
		Count(&n).Error
	return int(n), err
}

func (r *bookingRepository) frequencyCounts(tx *gorm.DB, brandID, learnerID int64, startsAt time.Time) (freqCounts, error) {
	var fc freqCounts
	ds, de := dayBounds(startsAt)
	ws, we := weekBounds(startsAt)
	ms, me := monthBounds(startsAt)
	var err error
	if fc.day, err = r.countBookingsInRange(tx, brandID, learnerID, ds, de); err != nil {
		return fc, apperr.ErrInternalF("统计每日预约失败", err)
	}
	if fc.week, err = r.countBookingsInRange(tx, brandID, learnerID, ws, we); err != nil {
		return fc, apperr.ErrInternalF("统计每周预约失败", err)
	}
	if fc.month, err = r.countBookingsInRange(tx, brandID, learnerID, ms, me); err != nil {
		return fc, apperr.ErrInternalF("统计每月预约失败", err)
	}
	var conc int64
	if err = tx.Model(&BookingModel{}).
		Where("brand_id = ? AND brand_learner_profile_id = ? AND status = 'booked'", brandID, learnerID).
		Count(&conc).Error; err != nil {
		return fc, apperr.ErrInternalF("统计未完成预约失败", err)
	}
	fc.concurrent = int(conc)
	return fc, nil
}

// freqExceeded 叠加 policy + product 限额取最严，判断是否超限。which 返回超限维度。
func freqExceeded(fc freqCounts, eff booking.EffectivePolicy, prod *EntitlementProductModel) (string, bool) {
	type dim struct {
		which string
		count int
		limit *int
	}
	var pd, pw, pm, pc *int
	if prod != nil {
		pd, pw, pm, pc = prod.DailyBookingLimit, prod.WeeklyBookingLimit, prod.MonthlyBookingLimit, prod.ConcurrentBookingLimit
	}
	dims := []dim{
		{"daily", fc.day, minLimitPtr(eff.DailyBookingLimit, pd)},
		{"weekly", fc.week, minLimitPtr(eff.WeeklyBookingLimit, pw)},
		{"monthly", fc.month, pm}, // policy 无月限，独家来自产品
		{"concurrent", fc.concurrent, minLimitPtr(eff.ConcurrentBookingLimit, pc)},
	}
	for _, d := range dims {
		if d.limit != nil && d.count >= *d.limit {
			return d.which, true
		}
	}
	return "", false
}

// ---- 权益可用性 / scope ----

func entitlementUsable(le *LearnerEntitlementModel, now time.Time) bool {
	if le.Status != string(entitlement.StatusActive) {
		return false
	}
	if !le.ExpiresAt.After(now) {
		return false
	}
	if le.TotalCredits != nil && le.RemainingCredits != nil && *le.RemainingCredits <= 0 {
		return false
	}
	return true
}

func (r *bookingRepository) entitlementScopeMatches(tx *gorm.DB, prod *EntitlementProductModel, sessLocation, sessCourse int64) (bool, error) {
	if prod.LocationScope == entitlement.ScopeSpecific {
		var n int64
		if err := tx.Table("entitlement_product_locations").
			Where("product_id = ? AND location_id = ?", prod.ID, sessLocation).Count(&n).Error; err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	if prod.CourseScope == entitlement.ScopeSpecific {
		var n int64
		if err := tx.Table("entitlement_product_courses").
			Where("product_id = ? AND course_id = ?", prod.ID, sessCourse).Count(&n).Error; err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	return true, nil
}

// loadCandidates 学员对某场次的可用权益（active/未过期/余额/scope 命中 + 频次未超），按 §5.7 排序。
func (r *bookingRepository) loadCandidates(tx *gorm.DB, brandID, learnerID, sessLocation, sessCourse int64, eff booking.EffectivePolicy, fc freqCounts, now time.Time) ([]booking.EntitlementCandidate, map[int64]*EntitlementProductModel, error) {
	var les []LearnerEntitlementModel
	if err := tx.Where("brand_id = ? AND brand_learner_profile_id = ? AND status = ? AND expires_at > ? AND (remaining_credits IS NULL OR remaining_credits > 0)",
		brandID, learnerID, string(entitlement.StatusActive), now).
		Order("expires_at ASC, id ASC").Find(&les).Error; err != nil {
		return nil, nil, apperr.ErrInternalF("查询学员权益失败", err)
	}
	prods := map[int64]*EntitlementProductModel{}
	var cands []booking.EntitlementCandidate
	for i := range les {
		le := les[i]
		prod, ok := prods[le.ProductID]
		if !ok {
			var pm EntitlementProductModel
			if err := tx.Where("id = ? AND brand_id = ?", le.ProductID, brandID).First(&pm).Error; err != nil {
				return nil, nil, apperr.ErrInternalF("查询权益产品失败", err)
			}
			prod = &pm
			prods[le.ProductID] = prod
		}
		matched, err := r.entitlementScopeMatches(tx, prod, sessLocation, sessCourse)
		if err != nil {
			return nil, nil, apperr.ErrInternalF("校验权益适用范围失败", err)
		}
		if !matched {
			continue
		}
		if _, exceeded := freqExceeded(fc, eff, prod); exceeded {
			continue
		}
		cands = append(cands, booking.EntitlementCandidate{
			EntitlementID:  le.ID,
			ProductType:    entitlement.ProductType(prod.ProductType),
			CourseSpecific: prod.CourseScope == entitlement.ScopeSpecific,
			ExpiresAt:      le.ExpiresAt,
		})
	}
	booking.SortCandidates(cands)
	return cands, prods, nil
}

// ---- TX-1 下单 ----

// placeBooking 下单核心（共享）：校容量 → 学员可预约 → 解析权益(auto/manual/none) → booked_count++
// → INSERT booking(指定 source) → 锁权益/hold/流水。调用方须已 SELECT FOR UPDATE 锁 sess 行，并自行
// 处理场次状态/窗口/scope 等前置 + 成功后的 audit。TX-1 代预约(staff_assisted) 与 13d 候补转正
// (waitlist_promotion) 共用，零逻辑漂移。
// assistedBy 为代预约员工 id（FK→brand_users）；C 端学员自助传 nil（assisted_by + txn operated_by 均 NULL，
// 学员身份靠 brand_learner_profile_id + audit 承载——assisted_by 是 brand_users FK，塞 learner id 会 23503）。
func (r *bookingRepository) placeBooking(tx *gorm.DB, sess *ClassSessionModel, eff booking.EffectivePolicy, brandID int64, assistedBy *int64, learnerID int64, mode booking.EntitlementMode, manualEntitlementID *int64, noEntitlementReason, source string, now time.Time) (*BookingModel, error) {
	if sess.BookedCount >= sess.Capacity {
		return nil, apperr.NewAppError(apperr.ErrSessionFull, "场次已满员", 409)
	}
	// 学员可预约。
	var prof BrandLearnerProfileModel
	if err := tx.Where("id = ? AND brand_id = ?", learnerID, brandID).First(&prof).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询学员失败", err)
	}
	if prof.Status != string(learner.StatusActive) {
		return nil, apperr.NewAppError(apperr.ErrLearnerNotBookable, "学员当前不可预约", 409)
	}

	// 解析权益（占位 / 自动 / 手动）。
	var chosen *LearnerEntitlementModel
	requiresFix := false
	var noReason *string
	switch mode {
	case booking.ModeNone:
		reason := strings.TrimSpace(noEntitlementReason)
		if reason == "" {
			return nil, apperr.NewAppError(apperr.ErrAssistedReasonRequired, "无权益占位须填写原因", 422)
		}
		requiresFix = true
		noReason = &reason
	case booking.ModeAuto, booking.ModeManual:
		fc, ferr := r.frequencyCounts(tx, brandID, learnerID, sess.StartsAt)
		if ferr != nil {
			return nil, ferr
		}
		// policy 级频次与具体权益无关：超限直接报频次（避免被误判成「无可用权益」）。
		if which, exceeded := freqExceeded(fc, eff, nil); exceeded {
			return nil, apperr.NewAppError(apperr.ErrBookingFrequencyExceeded, "预约频次超限", 409).
				WithDetails(map[string]any{"which": which})
		}
		var entID int64
		if mode == booking.ModeManual {
			if manualEntitlementID == nil || *manualEntitlementID <= 0 {
				return nil, apperr.NewAppError(apperr.ErrInvalidParam, "请指定权益", 400)
			}
			entID = *manualEntitlementID
		} else {
			cands, _, cerr := r.loadCandidates(tx, brandID, learnerID, sess.LocationID, sess.CourseID, eff, fc, now)
			if cerr != nil {
				return nil, cerr
			}
			best, ok := booking.SelectAuto(cands)
			if !ok {
				return nil, apperr.NewAppError(apperr.ErrEntitlementNoneAvailable, "该学员无可用权益", 409)
			}
			entID = best.EntitlementID
		}
		// 锁权益行 + 校验（auto 选中项也在锁后复验，挡 remaining 竞态）。
		var le LearnerEntitlementModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ? AND brand_learner_profile_id = ?", entID, brandID, learnerID).
			First(&le).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperr.NewAppError(apperr.ErrEntitlementNotFound, "权益不存在", 404)
			}
			return nil, apperr.ErrInternalF("查询权益失败", err)
		}
		if !entitlementUsable(&le, now) {
			return nil, apperr.NewAppError(apperr.ErrEntitlementNotUsable, "权益不可用（已过期/耗尽/冻结）", 422)
		}
		var prod EntitlementProductModel
		if err := tx.Where("id = ? AND brand_id = ?", le.ProductID, brandID).First(&prod).Error; err != nil {
			return nil, apperr.ErrInternalF("查询权益产品失败", err)
		}
		matched, merr := r.entitlementScopeMatches(tx, &prod, sess.LocationID, sess.CourseID)
		if merr != nil {
			return nil, apperr.ErrInternalF("校验权益适用范围失败", merr)
		}
		if !matched {
			return nil, apperr.NewAppError(apperr.ErrEntitlementScopeMismatch, "权益不适用于该场次", 422)
		}
		if which, exceeded := freqExceeded(fc, eff, &prod); exceeded {
			return nil, apperr.NewAppError(apperr.ErrBookingFrequencyExceeded, "预约频次超限", 409).
				WithDetails(map[string]any{"which": which})
		}
		chosen = &le
	default:
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的权益模式", 400)
	}

	// booked_count++（所有校验通过后才占位）。
	if err := tx.Model(&ClassSessionModel{}).Where("id = ?", sess.ID).
		Update("booked_count", gorm.Expr("booked_count + 1")).Error; err != nil {
		return nil, apperr.ErrInternalF("更新场次占用失败", err)
	}
	sess.BookedCount++
	// INSERT booking。
	bk := BookingModel{
		BrandID:                brandID,
		ClassSessionID:         sess.ID,
		BrandLearnerProfileID:  learnerID,
		Source:                 source,
		Status:                 string(booking.StatusBooked),
		BookedAt:               now,
		AssistedBy:             assistedBy,
		RequiresEntitlementFix: requiresFix,
		NoEntitlementReason:    noReason,
	}
	if err := tx.Create(&bk).Error; err != nil {
		if be := bookingConflictError(err); be != nil {
			return nil, be
		}
		return nil, apperr.ErrInternalF("创建预约失败", err)
	}

	// 权益路径：扣额 + hold + 流水。
	if chosen != nil {
		countBased := chosen.TotalCredits != nil && chosen.RemainingCredits != nil
		delta := 0
		var balanceAfter *int
		upd := map[string]interface{}{"locked_credits": chosen.LockedCredits + 1}
		settleRemaining := chosen.RemainingCredits
		if countBased {
			nr := *chosen.RemainingCredits - 1
			if nr < 0 {
				return nil, apperr.NewAppError(apperr.ErrEntitlementNotUsable, "权益余额不足", 422)
			}
			upd["remaining_credits"] = nr
			delta = -1
			balanceAfter = &nr
			settleRemaining = &nr
		}
		newStatus := entitlement.SettleStatus(entitlement.StatusActive, chosen.ExpiresAt, chosen.TotalCredits, settleRemaining, now)
		upd["status"] = string(newStatus)
		if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ?", chosen.ID).Updates(upd).Error; err != nil {
			return nil, apperr.ErrInternalF("锁定权益失败", err)
		}
		hold := EntitlementHoldModel{
			BrandID:               brandID,
			BookingID:             bk.ID,
			LearnerEntitlementID:  chosen.ID,
			BrandLearnerProfileID: learnerID,
			Credits:               1,
			Status:                string(booking.HoldStatusHeld),
			HeldAt:                now,
		}
		if err := tx.Create(&hold).Error; err != nil {
			if be := bookingConflictError(err); be != nil {
				return nil, be
			}
			return nil, apperr.ErrInternalF("锁定权益失败", err)
		}
		if err := insertBookingTransaction(tx, brandID, chosen.ID, learnerID, &bk.ID, &hold.ID, nil, entitlement.ActionHold, delta, balanceAfter, "预约锁定", assistedBy); err != nil {
			return nil, err
		}
	}
	return &bk, nil
}

func (r *bookingRepository) Create(ctx context.Context, in booking.CreateInput) (*booking.Booking, error) {
	var createdID int64
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁场次行 + 前置校验（scope/状态/窗口）。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", in.ClassSessionID, in.BrandID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
			}
			return apperr.ErrInternalF("查询场次失败", err)
		}
		// data_scope：越权场次按不存在处理（不泄漏存在性）。
		if in.ScopeLocationIDs != nil && !int64InSlice(sess.LocationID, in.ScopeLocationIDs) {
			return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
		}
		if sess.Status != "scheduled" {
			return apperr.NewAppError(apperr.ErrSessionNotBookable, "场次当前不可预约", 409)
		}
		eff, err := r.resolveEffectivePolicy(tx, in.BrandID, sess.LocationID, sess.ID)
		if err != nil {
			return err
		}
		if !booking.WithinBookingWindow(now, sess.StartsAt, eff) {
			return apperr.NewAppError(apperr.ErrBookingWindowClosed, "当前不在可预约时间窗口内", 409)
		}
		// 2-6. 下单核心。staff 代预约：assisted_by = 员工。
		actor := in.ActorID
		bk, err := r.placeBooking(tx, &sess, eff, in.BrandID, &actor, in.BrandLearnerProfileID,
			in.EntitlementMode, in.LearnerEntitlementID, in.NoEntitlementReason, string(booking.SourceStaffAssisted), now)
		if err != nil {
			return err
		}
		createdID = bk.ID
		// 7. audit。
		action := "booking_created"
		if bk.RequiresEntitlementFix {
			action = "booking_created_no_entitlement"
		}
		return writeBookingLog(tx, in.BrandID, in.ActorID, action, bk.ID, nil, bk)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, createdID)
}

// ---- TX-L1 学员自助下单（Batch 14a）----

// hasOverlappingBooking 报告该学员是否已有未取消预约、其场次时段与 [startsAt,endsAt) 重叠（排除本场次，§22.1）。
// 重叠定义：existing.starts < new.ends AND existing.ends > new.starts。
func (r *bookingRepository) hasOverlappingBooking(tx *gorm.DB, brandID, learnerID, sessionID int64, startsAt, endsAt time.Time) (bool, error) {
	var n int64
	err := tx.Table("bookings b").
		Joins("JOIN class_sessions cs ON cs.id = b.class_session_id").
		Where("b.brand_id = ? AND b.brand_learner_profile_id = ? AND b.status <> 'cancelled'", brandID, learnerID).
		Where("cs.id <> ?", sessionID).
		Where("cs.starts_at < ? AND cs.ends_at > ?", endsAt, startsAt).
		Count(&n).Error
	if err != nil {
		return false, apperr.ErrInternalF("校验时间冲突失败", err)
	}
	return n > 0, nil
}

// CreateByLearner C 端学员自助下单（source=learner_self_service，assisted_by NULL，mode=auto，无 data_scope）。
// 复用 placeBooking 核心；前置加 §22.1 跨场次时间重叠校验；audit actor=learner。
func (r *bookingRepository) CreateByLearner(ctx context.Context, in booking.LearnerCreateInput) (*booking.Booking, error) {
	var createdID int64
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁场次行 + 状态/窗口校验。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", in.ClassSessionID, in.BrandID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
			}
			return apperr.ErrInternalF("查询场次失败", err)
		}
		if sess.Status != "scheduled" {
			return apperr.NewAppError(apperr.ErrSessionNotBookable, "场次当前不可预约", 409)
		}
		eff, err := r.resolveEffectivePolicy(tx, in.BrandID, sess.LocationID, sess.ID)
		if err != nil {
			return err
		}
		if !booking.WithinBookingWindow(now, sess.StartsAt, eff) {
			return apperr.NewAppError(apperr.ErrBookingWindowClosed, "当前不在可预约时间窗口内", 409)
		}
		// 2. 跨场次时间重叠校验（§22.1，仅学员路径）。
		conflict, cerr := r.hasOverlappingBooking(tx, in.BrandID, in.BrandLearnerProfileID, sess.ID, sess.StartsAt, sess.EndsAt)
		if cerr != nil {
			return cerr
		}
		if conflict {
			return apperr.NewAppError(apperr.ErrBookingTimeConflict, "同一时间已有预约，时间冲突", 409)
		}
		// 3. 下单核心：assisted_by=nil（自助）、auto、source=learner_self_service。
		bk, err := r.placeBooking(tx, &sess, eff, in.BrandID, nil, in.BrandLearnerProfileID,
			booking.ModeAuto, nil, "", string(booking.SourceLearnerSelfService), now)
		if err != nil {
			return err
		}
		createdID = bk.ID
		// 4. audit（actor=learner）。
		return writeBookingLogAs(tx, audit.Actor{Type: audit.ActorLearner, ID: in.BrandLearnerProfileID},
			in.BrandID, "booking_created", bk.ID, nil, bk)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, createdID)
}

// ---- TX-2 代取消 ----

func (r *bookingRepository) Cancel(ctx context.Context, brandID, actorID, id int64, reason string) (*booking.Booking, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 预读取场次 id（未锁；权威校验在锁后）。
		var pre BookingModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		// 锁序：先场次后预约（与下单/场次取消一致）。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", pre.ClassSessionID).First(&sess).Error; err != nil {
			return apperr.ErrInternalF("查询场次失败", err)
		}
		var bk BookingModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", id, brandID).First(&bk).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		if bk.Status != string(booking.StatusBooked) {
			return apperr.NewAppError(apperr.ErrBookingNotCancellable, "该预约当前不可取消", 409)
		}
		eff, perr := r.resolveEffectivePolicy(tx, brandID, sess.LocationID, sess.ID)
		if perr != nil {
			return perr
		}
		if !eff.AllowCancel {
			return apperr.NewAppError(apperr.ErrBookingCancelNotAllowed, "该场次不允许取消", 409)
		}
		if booking.CancelDeadlinePassed(now, sess.StartsAt, eff) {
			return apperr.NewAppError(apperr.ErrBookingCancelDeadlinePassed, "已超过取消截止时间", 409)
		}
		if err := r.applyCancel(tx, &bk, &sess, &actorID, booking.CancelSourceStaff, reason, eff.ReleaseOnCancel, now); err != nil {
			return err
		}
		after := bk
		after.Status = string(booking.StatusCancelled)
		return writeBookingLog(tx, brandID, actorID, "booking_cancelled", bk.ID, &bk, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

// CancelByLearner C 端学员自助取消（Batch 14a）：tx 内校所有权 + cancel_source=learner + cancelled_by NULL。
// 镜像 Cancel，但 booking 锁定条件带 brand_learner_profile_id（越权按不存在），actorBy=nil，audit actor=learner。
func (r *bookingRepository) CancelByLearner(ctx context.Context, brandID, profileID, id int64, reason string) (*booking.Booking, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 预读 + 所有权校验（越权/不存在均按不存在，不泄漏）。
		var pre BookingModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		if pre.BrandLearnerProfileID != profileID {
			return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
		}
		// 锁序：先场次后预约（与下单/代取消一致）。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", pre.ClassSessionID).First(&sess).Error; err != nil {
			return apperr.ErrInternalF("查询场次失败", err)
		}
		// booking 锁定带 profile 条件，tx 内再校所有权（防 TOCTOU）。
		var bk BookingModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ? AND brand_learner_profile_id = ?", id, brandID, profileID).First(&bk).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		if bk.Status != string(booking.StatusBooked) {
			return apperr.NewAppError(apperr.ErrBookingNotCancellable, "该预约当前不可取消", 409)
		}
		eff, perr := r.resolveEffectivePolicy(tx, brandID, sess.LocationID, sess.ID)
		if perr != nil {
			return perr
		}
		if !eff.AllowCancel {
			return apperr.NewAppError(apperr.ErrBookingCancelNotAllowed, "该场次不允许取消", 409)
		}
		if booking.CancelDeadlinePassed(now, sess.StartsAt, eff) {
			return apperr.NewAppError(apperr.ErrBookingCancelDeadlinePassed, "已超过取消截止时间", 409)
		}
		// 学员自助：actorBy=nil（cancelled_by + txn operated_by NULL），cancel_source=learner。
		if err := r.applyCancel(tx, &bk, &sess, nil, booking.CancelSourceLearner, reason, eff.ReleaseOnCancel, now); err != nil {
			return err
		}
		after := bk
		after.Status = string(booking.StatusCancelled)
		return writeBookingLogAs(tx, audit.Actor{Type: audit.ActorLearner, ID: profileID}, brandID, "booking_cancelled", bk.ID, &bk, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

// applyCancel 把单条 booked 预约置 cancelled + 退名额 + 处理 hold（release / forfeit）。
// 调用方须已锁 session 行与 booking 行。release=false 时走 forfeit（hold→consumed，权益不回退）。
// actorBy 为操作员工 id（FK→brand_users）；C 端学员自助取消传 nil（cancelled_by + txn operated_by NULL）。
func (r *bookingRepository) applyCancel(tx *gorm.DB, bk *BookingModel, sess *ClassSessionModel, actorBy *int64, cancelSource, reason string, release bool, now time.Time) error {
	cs := cancelSource
	upd := map[string]interface{}{
		"status":        string(booking.StatusCancelled),
		"cancelled_at":  now,
		"cancelled_by":  actorBy,
		"cancel_source": cs,
		"cancel_reason": strings.TrimSpace(reason),
	}
	if err := tx.Model(&BookingModel{}).Where("id = ?", bk.ID).Updates(upd).Error; err != nil {
		return apperr.ErrInternalF("取消预约失败", err)
	}
	if sess.BookedCount > 0 {
		if err := tx.Model(&ClassSessionModel{}).Where("id = ?", sess.ID).
			Update("booked_count", gorm.Expr("booked_count - 1")).Error; err != nil {
			return apperr.ErrInternalF("更新场次占用失败", err)
		}
		sess.BookedCount--
	}
	return settleHoldOnCancel(tx, bk.BrandID, bk.ID, bk.BrandLearnerProfileID, actorBy, release, now)
}

// settleHoldOnCancel 处理取消时的 hold：release=true 释放权益（locked--/remaining++/re-settle），
// release=false 没收（hold→consumed，locked--/consumed++，remaining 不回退）。无 hold（占位预约）直接返回。
// 自由函数（同包）：TX-2 代取消与 TX-3 场次取消级联共用。
func settleHoldOnCancel(tx *gorm.DB, brandID, bookingID, learnerID int64, actorBy *int64, release bool, now time.Time) error {
	var hold EntitlementHoldModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("booking_id = ? AND status = ?", bookingID, string(booking.HoldStatusHeld)).First(&hold).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // 占位预约无 hold。
	}
	if err != nil {
		return apperr.ErrInternalF("查询权益锁定失败", err)
	}
	var le LearnerEntitlementModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", hold.LearnerEntitlementID).First(&le).Error; err != nil {
		return apperr.ErrInternalF("查询权益失败", err)
	}
	countBased := le.TotalCredits != nil && le.RemainingCredits != nil
	leUpd := map[string]interface{}{}
	if le.LockedCredits > 0 {
		leUpd["locked_credits"] = le.LockedCredits - 1
	}
	if release {
		delta := 0
		var balanceAfter *int
		settleRemaining := le.RemainingCredits
		if countBased {
			nr := *le.RemainingCredits + 1
			leUpd["remaining_credits"] = nr
			delta = 1
			balanceAfter = &nr
			settleRemaining = &nr
		}
		leUpd["status"] = string(entitlement.SettleStatus(entitlement.Status(le.Status), le.ExpiresAt, le.TotalCredits, settleRemaining, now))
		if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ?", le.ID).Updates(leUpd).Error; err != nil {
			return apperr.ErrInternalF("释放权益失败", err)
		}
		if err := tx.Model(&EntitlementHoldModel{}).Where("id = ?", hold.ID).
			Updates(map[string]interface{}{"status": string(booking.HoldStatusReleased), "released_at": now}).Error; err != nil {
			return apperr.ErrInternalF("释放权益锁定失败", err)
		}
		return insertBookingTransaction(tx, brandID, le.ID, learnerID, &bookingID, &hold.ID, nil, entitlement.ActionRelease, delta, balanceAfter, "取消释放", actorBy)
	}
	// forfeit：没收已扣的课时。
	leUpd["consumed_credits"] = le.ConsumedCredits + 1
	if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ?", le.ID).Updates(leUpd).Error; err != nil {
		return apperr.ErrInternalF("没收权益失败", err)
	}
	if err := tx.Model(&EntitlementHoldModel{}).Where("id = ?", hold.ID).
		Updates(map[string]interface{}{"status": string(booking.HoldStatusConsumed), "consumed_at": now}).Error; err != nil {
		return apperr.ErrInternalF("没收权益锁定失败", err)
	}
	return insertBookingTransaction(tx, brandID, le.ID, learnerID, &bookingID, &hold.ID, nil, entitlement.ActionConsume, 0, le.RemainingCredits, "取消没收", actorBy)
}

// ---- TX-A 签到 / TX-C 爽约：履约结算 hold（Batch 13e）----

// settleHoldForOutcome 履约结算 hold（签到到课 / 爽约扣课 / 爽约退课）。与取消的 settleHoldOnCancel
// 并列但语义不同：本函数用于「到课/爽约」终态，consume 路径额外写 entitlement_consumptions（unique(hold)
// 防重复消费同 hold）并把 txn 关联 consumption_id。无 hold（占位预约）→ 返回 nil，不结算。
//   - consume=true：held→consumed / locked-- / consumed++（remaining 不变，下单时已扣）+ consumption 行 + txn(action)
//   - consume=false：held→released / locked-- / remaining++ + txn(release)（爽约不扣课，无 consumption）
// consumptionType / attendanceID 仅 consume 路径用；release 路径忽略。
func settleHoldForOutcome(tx *gorm.DB, brandID, bookingID, learnerID, actorID int64, consume bool,
	consumptionType booking.ConsumptionType, attendanceID *int64, action entitlement.Action, note string, now time.Time) error {
	var hold EntitlementHoldModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("booking_id = ? AND status = ?", bookingID, string(booking.HoldStatusHeld)).First(&hold).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // 占位预约无 hold，不结算。
	}
	if err != nil {
		return apperr.ErrInternalF("查询权益锁定失败", err)
	}
	var le LearnerEntitlementModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", hold.LearnerEntitlementID).First(&le).Error; err != nil {
		return apperr.ErrInternalF("查询权益失败", err)
	}
	countBased := le.TotalCredits != nil && le.RemainingCredits != nil
	leUpd := map[string]interface{}{}
	if le.LockedCredits > 0 {
		leUpd["locked_credits"] = le.LockedCredits - 1
	}
	actor := actorID

	if !consume {
		// 释放（爽约不扣课）：locked--/remaining++/re-settle/hold→released/txn release。
		delta := 0
		var balanceAfter *int
		settleRemaining := le.RemainingCredits
		if countBased {
			nr := *le.RemainingCredits + 1
			leUpd["remaining_credits"] = nr
			delta = 1
			balanceAfter = &nr
			settleRemaining = &nr
		}
		leUpd["status"] = string(entitlement.SettleStatus(entitlement.Status(le.Status), le.ExpiresAt, le.TotalCredits, settleRemaining, now))
		if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ?", le.ID).Updates(leUpd).Error; err != nil {
			return apperr.ErrInternalF("释放权益失败", err)
		}
		if err := tx.Model(&EntitlementHoldModel{}).Where("id = ?", hold.ID).
			Updates(map[string]interface{}{"status": string(booking.HoldStatusReleased), "released_at": now}).Error; err != nil {
			return apperr.ErrInternalF("释放权益锁定失败", err)
		}
		return insertBookingTransaction(tx, brandID, le.ID, learnerID, &bookingID, &hold.ID, nil, entitlement.ActionRelease, delta, balanceAfter, note, &actor)
	}

	// 消耗（到课 / 爽约扣课）：locked--/consumed++（remaining 不变）/re-settle/hold→consumed。
	leUpd["consumed_credits"] = le.ConsumedCredits + 1
	leUpd["status"] = string(entitlement.SettleStatus(entitlement.Status(le.Status), le.ExpiresAt, le.TotalCredits, le.RemainingCredits, now))
	if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ?", le.ID).Updates(leUpd).Error; err != nil {
		return apperr.ErrInternalF("消耗权益失败", err)
	}
	if err := tx.Model(&EntitlementHoldModel{}).Where("id = ?", hold.ID).
		Updates(map[string]interface{}{"status": string(booking.HoldStatusConsumed), "consumed_at": now}).Error; err != nil {
		return apperr.ErrInternalF("消耗权益锁定失败", err)
	}
	// entitlement_consumptions（unique(entitlement_hold_id) 防重复消费同 hold → 23505）。
	cons := EntitlementConsumptionModel{
		BrandID: brandID, EntitlementHoldID: &hold.ID, LearnerEntitlementID: le.ID, BookingID: bookingID,
		AttendanceID: attendanceID, BrandLearnerProfileID: learnerID, Credits: hold.Credits,
		ConsumptionType: string(consumptionType), ConsumedAt: now, OperatedBy: &actor,
	}
	if err := tx.Create(&cons).Error; err != nil {
		if name, ok := uniqueConstraint(err); ok && strings.Contains(name, "consumptions_hold") {
			return apperr.NewAppError(apperr.ErrAttendanceAlreadyMarked, "该预约已结算", 409)
		}
		return apperr.ErrInternalF("写入权益消耗失败", err)
	}
	return insertBookingTransaction(tx, brandID, le.ID, learnerID, &bookingID, &hold.ID, &cons.ID, action, 0, le.RemainingCredits, note, &actor)
}

// ---- TX-A 签到（标到课）----

// Attend booked|pending_no_show → attended + attendance_records + hold consume + session_records。
// 占位预约（requires_entitlement_fix，无 hold）仍 attended + records，但不消费（§13.2 留人工补）。
func (r *bookingRepository) Attend(ctx context.Context, brandID, actorID, id int64, note string) (*booking.Booking, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pre BookingModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		// 锁序：先场次后预约（与下单/取消一致）。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", pre.ClassSessionID).First(&sess).Error; err != nil {
			return apperr.ErrInternalF("查询场次失败", err)
		}
		var bk BookingModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", id, brandID).First(&bk).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		if bk.Status == string(booking.StatusAttended) {
			return apperr.NewAppError(apperr.ErrAttendanceAlreadyMarked, "该预约已签到", 409)
		}
		if !booking.CanAttend(booking.Status(bk.Status)) {
			return apperr.NewAppError(apperr.ErrBookingNotAttendable, "该预约当前不可签到", 409)
		}
		if sess.Status == "cancelled" {
			return apperr.NewAppError(apperr.ErrBookingNotAttendable, "场次已取消，不可签到", 409)
		}
		after := bk
		after.Status = string(booking.StatusAttended)
		if err := tx.Model(&BookingModel{}).Where("id = ?", bk.ID).Update("status", after.Status).Error; err != nil {
			return apperr.ErrInternalF("更新预约状态失败", err)
		}
		var notePtr *string
		if n := strings.TrimSpace(note); n != "" {
			notePtr = &n
		}
		// attendance_records（unique(booking) 防重签 → 23505）。
		att := AttendanceRecordModel{
			BrandID: brandID, BookingID: bk.ID, ClassSessionID: sess.ID,
			BrandLearnerProfileID: bk.BrandLearnerProfileID, MarkedBy: &actorID, AttendedAt: now, Note: notePtr,
		}
		if err := tx.Create(&att).Error; err != nil {
			if name, ok := uniqueConstraint(err); ok && strings.Contains(name, "attendance_records_booking") {
				return apperr.NewAppError(apperr.ErrAttendanceAlreadyMarked, "该预约已签到", 409)
			}
			return apperr.ErrInternalF("写入签到记录失败", err)
		}
		// hold 收口：consume（占位无 hold→跳过，不消费）。
		if err := settleHoldForOutcome(tx, brandID, bk.ID, bk.BrandLearnerProfileID, actorID, true,
			booking.ConsumptionAttendance, &att.ID, entitlement.ActionConsume, "到课消费", now); err != nil {
			return err
		}
		// session_records（履约）。
		var instrPtr *int64
		if sess.InstructorProfileID > 0 {
			instr := sess.InstructorProfileID
			instrPtr = &instr
		}
		rec := SessionRecordModel{
			BrandID: brandID, ClassSessionID: sess.ID, BookingID: bk.ID, AttendanceID: &att.ID,
			BrandLearnerProfileID: bk.BrandLearnerProfileID, InstructorProfileID: instrPtr,
			RecordType: string(booking.RecordAttendance), Note: notePtr,
		}
		if err := tx.Create(&rec).Error; err != nil {
			return apperr.ErrInternalF("写入履约记录失败", err)
		}
		return writeBookingLog(tx, brandID, actorID, "booking_attended", bk.ID, &bk, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

// ---- TX-B 结束场次 ----

// EndSession scheduled|in_progress → completed + 未签到的 booked 批量 → pending_no_show（hold/booked_count 不动）。
// completed 场次不可再取消（class_session.Cancel 仅允 scheduled/in_progress），故 pending_no_show 永不被 TX-3 触及。
func (r *bookingRepository) EndSession(ctx context.Context, brandID, actorID, sessionID int64, scopeLocationIDs []int64) (*booking.EndSessionResult, error) {
	var pendingCount int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", sessionID, brandID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
			}
			return apperr.ErrInternalF("查询场次失败", err)
		}
		// data_scope：越权场次按不存在处理（不泄漏存在性）。
		if scopeLocationIDs != nil && !int64InSlice(sess.LocationID, scopeLocationIDs) {
			return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
		}
		if sess.Status != "scheduled" && sess.Status != "in_progress" {
			return apperr.NewAppError(apperr.ErrSessionNotEndable, "仅可结束未开始或进行中的场次", 409)
		}
		if err := tx.Model(&ClassSessionModel{}).Where("id = ?", sessionID).
			Update("status", "completed").Error; err != nil {
			return apperr.ErrInternalF("更新场次状态失败", err)
		}
		res := tx.Model(&BookingModel{}).
			Where("class_session_id = ? AND status = ?", sessionID, string(booking.StatusBooked)).
			Update("status", string(booking.StatusPendingNoShow))
		if res.Error != nil {
			return apperr.ErrInternalF("批量转待爽约失败", res.Error)
		}
		pendingCount = res.RowsAffected
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "session_ended",
			Target:  audit.Target{Type: "class_session", ID: sessionID},
			After:   map[string]any{"pending_no_show_count": pendingCount},
		})
	})
	if err != nil {
		return nil, err
	}
	return &booking.EndSessionResult{SessionID: sessionID, Status: "completed", PendingNoShowCount: int(pendingCount)}, nil
}

// ---- TX-C 确认爽约 ----

// ConfirmNoShow pending_no_show → no_show + 按 policy no_show_consumes_entitlement consume/release hold
// + session_records(no_show)。占位预约（无 hold）只置 no_show + 履约记录，不结算。
func (r *bookingRepository) ConfirmNoShow(ctx context.Context, brandID, actorID, id int64, reason string) (*booking.Booking, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pre BookingModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		// 锁序：先场次后预约（与下单/取消/签到一致）。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", pre.ClassSessionID).First(&sess).Error; err != nil {
			return apperr.ErrInternalF("查询场次失败", err)
		}
		var bk BookingModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", id, brandID).First(&bk).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
			}
			return apperr.ErrInternalF("查询预约失败", err)
		}
		if !booking.CanConfirmNoShow(booking.Status(bk.Status)) {
			return apperr.NewAppError(apperr.ErrBookingNotConfirmable, "该预约当前不可确认爽约", 409)
		}
		eff, perr := r.resolveEffectivePolicy(tx, brandID, sess.LocationID, sess.ID)
		if perr != nil {
			return perr
		}
		after := bk
		after.Status = string(booking.StatusNoShow)
		if err := tx.Model(&BookingModel{}).Where("id = ?", bk.ID).Update("status", after.Status).Error; err != nil {
			return apperr.ErrInternalF("更新预约状态失败", err)
		}
		// hold 收口：扣课（policy true）或退课（policy false）；占位无 hold→跳过。
		if eff.NoShowConsumesEntitlement {
			if err := settleHoldForOutcome(tx, brandID, bk.ID, bk.BrandLearnerProfileID, actorID, true,
				booking.ConsumptionNoShow, nil, entitlement.ActionNoShowConsume, "爽约扣课", now); err != nil {
				return err
			}
		} else {
			if err := settleHoldForOutcome(tx, brandID, bk.ID, bk.BrandLearnerProfileID, actorID, false,
				booking.ConsumptionNoShow, nil, entitlement.ActionRelease, "爽约释放", now); err != nil {
				return err
			}
		}
		// session_records(no_show, note=处理原因)。
		var notePtr *string
		if n := strings.TrimSpace(reason); n != "" {
			notePtr = &n
		}
		var instrPtr *int64
		if sess.InstructorProfileID > 0 {
			instr := sess.InstructorProfileID
			instrPtr = &instr
		}
		rec := SessionRecordModel{
			BrandID: brandID, ClassSessionID: sess.ID, BookingID: bk.ID, AttendanceID: nil,
			BrandLearnerProfileID: bk.BrandLearnerProfileID, InstructorProfileID: instrPtr,
			RecordType: string(booking.RecordNoShow), Note: notePtr,
		}
		if err := tx.Create(&rec).Error; err != nil {
			return apperr.ErrInternalF("写入履约记录失败", err)
		}
		return writeBookingLog(tx, brandID, actorID, "booking_no_show", bk.ID, &bk, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

// ---- usable-entitlements ----

func (r *bookingRepository) UsableEntitlements(ctx context.Context, brandID, sessionID, learnerID int64, scopeLocationIDs []int64) ([]*booking.UsableEntitlement, error) {
	now := time.Now().UTC()
	var out []*booking.UsableEntitlement
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess ClassSessionModel
		if err := tx.Where("id = ? AND brand_id = ?", sessionID, brandID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
			}
			return apperr.ErrInternalF("查询场次失败", err)
		}
		if scopeLocationIDs != nil && !int64InSlice(sess.LocationID, scopeLocationIDs) {
			return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
		}
		var cnt int64
		if err := tx.Model(&BrandLearnerProfileModel{}).Where("id = ? AND brand_id = ?", learnerID, brandID).Count(&cnt).Error; err != nil {
			return apperr.ErrInternalF("查询学员失败", err)
		}
		if cnt == 0 {
			return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
		}
		eff, perr := r.resolveEffectivePolicy(tx, brandID, sess.LocationID, sess.ID)
		if perr != nil {
			return perr
		}
		fc, ferr := r.frequencyCounts(tx, brandID, learnerID, sess.StartsAt)
		if ferr != nil {
			return ferr
		}
		cands, prods, cerr := r.loadCandidates(tx, brandID, learnerID, sess.LocationID, sess.CourseID, eff, fc, now)
		if cerr != nil {
			return cerr
		}
		// 需要 remaining：回查 entitlement 行（candidates 已排序）。
		for i, c := range cands {
			var le LearnerEntitlementModel
			if err := tx.Where("id = ?", c.EntitlementID).First(&le).Error; err != nil {
				return apperr.ErrInternalF("查询权益失败", err)
			}
			name := ""
			if p := prods[le.ProductID]; p != nil {
				name = p.Name
			}
			out = append(out, &booking.UsableEntitlement{
				EntitlementID:    le.ID,
				ProductName:      name,
				ProductType:      c.ProductType,
				RemainingCredits: le.RemainingCredits,
				ExpiresAt:        le.ExpiresAt,
				AutoSelected:     i == 0,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---- booking policy ----

func (r *bookingRepository) GetDefaultPolicy(ctx context.Context, brandID int64) (*booking.Policy, error) {
	var m BrandBookingPolicyModel
	err := r.db.WithContext(ctx).Where("brand_id = ? AND location_id IS NULL", brandID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		p := booking.DefaultPolicy()
		return &p, nil
	}
	if err != nil {
		return nil, apperr.ErrInternalF("查询预约策略失败", err)
	}
	p := policyFromModel(&m)
	return &p, nil
}

func (r *bookingRepository) UpsertDefaultPolicy(ctx context.Context, brandID, actorID int64, p booking.Policy) (*booking.Policy, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m BrandBookingPolicyModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("brand_id = ? AND location_id IS NULL", brandID).First(&m).Error
		fields := map[string]interface{}{
			"book_ahead_max_minutes":       p.BookAheadMaxMinutes,
			"book_ahead_min_minutes":       p.BookAheadMinMinutes,
			"cancel_deadline_minutes":      p.CancelDeadlineMinutes,
			"release_on_cancel":            p.ReleaseOnCancel,
			"no_show_consumes_entitlement": p.NoShowConsumesEntitlement,
			"daily_booking_limit":          p.DailyBookingLimit,
			"weekly_booking_limit":         p.WeeklyBookingLimit,
			"concurrent_booking_limit":     p.ConcurrentBookingLimit,
			"allow_waitlist":               p.AllowWaitlist,
			"waitlist_limit":               p.WaitlistLimit,
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := BrandBookingPolicyModel{
				BrandID:                   brandID,
				BookAheadMaxMinutes:       p.BookAheadMaxMinutes,
				BookAheadMinMinutes:       p.BookAheadMinMinutes,
				CancelDeadlineMinutes:     p.CancelDeadlineMinutes,
				ReleaseOnCancel:           p.ReleaseOnCancel,
				NoShowConsumesEntitlement: p.NoShowConsumesEntitlement,
				DailyBookingLimit:         p.DailyBookingLimit,
				WeeklyBookingLimit:        p.WeeklyBookingLimit,
				ConcurrentBookingLimit:    p.ConcurrentBookingLimit,
				AllowWaitlist:             p.AllowWaitlist,
				WaitlistLimit:             p.WaitlistLimit,
			}
			if err := tx.Create(&row).Error; err != nil {
				return apperr.ErrInternalF("创建预约策略失败", err)
			}
			return writeBookingPolicyLog(tx, brandID, actorID, "booking_policy_updated", row.ID, nil, &row)
		}
		if err != nil {
			return apperr.ErrInternalF("查询预约策略失败", err)
		}
		before := m
		if err := tx.Model(&BrandBookingPolicyModel{}).Where("id = ?", m.ID).Updates(fields).Error; err != nil {
			return apperr.ErrInternalF("更新预约策略失败", err)
		}
		after := m
		return writeBookingPolicyLog(tx, brandID, actorID, "booking_policy_updated", m.ID, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetDefaultPolicy(ctx, brandID)
}

// ---- 共享辅助 ----

// insertBookingTransaction 写权益流水（带 booking_id / hold_id 关联，区别于 entitlement 包的精简版）。
func insertBookingTransaction(tx *gorm.DB, brandID, entitlementID, learnerID int64, bookingID, holdID, consumptionID *int64, action entitlement.Action, delta int, balanceAfter *int, note string, operatedBy *int64) error {
	row := EntitlementTransactionModel{
		BrandID:               brandID,
		LearnerEntitlementID:  entitlementID,
		BrandLearnerProfileID: learnerID,
		BookingID:             bookingID,
		HoldID:                holdID,
		ConsumptionID:         consumptionID,
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

func writeBookingLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *BookingModel) error {
	return writeBookingLogAs(tx, audit.Actor{Type: audit.ActorBrandUser, ID: actorID}, brandID, action, id, before, after)
}

// writeBookingLogAs 同 writeBookingLog 但 actor 可指定（C 端学员自助传 {ActorLearner, profileID}，Batch 14a）。
func writeBookingLogAs(tx *gorm.DB, actor audit.Actor, brandID int64, action string, id int64, before, after *BookingModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   actor,
		Action:  action,
		Target:  audit.Target{Type: "booking", ID: id},
		Before:  before,
		After:   after,
	})
}

func writeBookingPolicyLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *BrandBookingPolicyModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "brand_booking_policy", ID: id},
		Before:  before,
		After:   after,
	})
}
