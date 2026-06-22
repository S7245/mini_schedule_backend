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

func (r *bookingRepository) Create(ctx context.Context, in booking.CreateInput) (*booking.Booking, error) {
	var createdID int64
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁场次行。
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
		if sess.BookedCount >= sess.Capacity {
			return apperr.NewAppError(apperr.ErrSessionFull, "场次已满员", 409)
		}
		// 2. 学员可预约。
		var prof BrandLearnerProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", in.BrandLearnerProfileID, in.BrandID).First(&prof).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
			}
			return apperr.ErrInternalF("查询学员失败", err)
		}
		if prof.Status != string(learner.StatusActive) {
			return apperr.NewAppError(apperr.ErrLearnerNotBookable, "学员当前不可预约", 409)
		}

		// 3. 解析权益（占位 / 自动 / 手动）。
		var chosen *LearnerEntitlementModel
		var chosenProd *EntitlementProductModel
		requiresFix := false
		var noReason *string
		switch in.EntitlementMode {
		case booking.ModeNone:
			reason := strings.TrimSpace(in.NoEntitlementReason)
			if reason == "" {
				return apperr.NewAppError(apperr.ErrAssistedReasonRequired, "无权益占位须填写原因", 422)
			}
			requiresFix = true
			noReason = &reason
		case booking.ModeAuto, booking.ModeManual:
			fc, ferr := r.frequencyCounts(tx, in.BrandID, in.BrandLearnerProfileID, sess.StartsAt)
			if ferr != nil {
				return ferr
			}
			// policy 级频次与具体权益无关：超限直接报频次（避免被误判成「无可用权益」）。
			if which, exceeded := freqExceeded(fc, eff, nil); exceeded {
				return apperr.NewAppError(apperr.ErrBookingFrequencyExceeded, "预约频次超限", 409).
					WithDetails(map[string]any{"which": which})
			}
			var entID int64
			if in.EntitlementMode == booking.ModeManual {
				if in.LearnerEntitlementID == nil || *in.LearnerEntitlementID <= 0 {
					return apperr.NewAppError(apperr.ErrInvalidParam, "请指定权益", 400)
				}
				entID = *in.LearnerEntitlementID
			} else {
				cands, _, cerr := r.loadCandidates(tx, in.BrandID, in.BrandLearnerProfileID, sess.LocationID, sess.CourseID, eff, fc, now)
				if cerr != nil {
					return cerr
				}
				best, ok := booking.SelectAuto(cands)
				if !ok {
					return apperr.NewAppError(apperr.ErrEntitlementNoneAvailable, "该学员无可用权益", 409)
				}
				entID = best.EntitlementID
			}
			// 锁权益行 + 校验（auto 选中项也在锁后复验，挡 remaining 竞态）。
			var le LearnerEntitlementModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ? AND brand_id = ? AND brand_learner_profile_id = ?", entID, in.BrandID, in.BrandLearnerProfileID).
				First(&le).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperr.NewAppError(apperr.ErrEntitlementNotFound, "权益不存在", 404)
				}
				return apperr.ErrInternalF("查询权益失败", err)
			}
			if !entitlementUsable(&le, now) {
				return apperr.NewAppError(apperr.ErrEntitlementNotUsable, "权益不可用（已过期/耗尽/冻结）", 422)
			}
			var prod EntitlementProductModel
			if err := tx.Where("id = ? AND brand_id = ?", le.ProductID, in.BrandID).First(&prod).Error; err != nil {
				return apperr.ErrInternalF("查询权益产品失败", err)
			}
			matched, merr := r.entitlementScopeMatches(tx, &prod, sess.LocationID, sess.CourseID)
			if merr != nil {
				return apperr.ErrInternalF("校验权益适用范围失败", merr)
			}
			if !matched {
				return apperr.NewAppError(apperr.ErrEntitlementScopeMismatch, "权益不适用于该场次", 422)
			}
			if which, exceeded := freqExceeded(fc, eff, &prod); exceeded {
				return apperr.NewAppError(apperr.ErrBookingFrequencyExceeded, "预约频次超限", 409).
					WithDetails(map[string]any{"which": which})
			}
			chosen = &le
			chosenProd = &prod
		default:
			return apperr.NewAppError(apperr.ErrInvalidParam, "无效的权益模式", 400)
		}

		// 4. booked_count++（所有校验通过后才占位）。
		if err := tx.Model(&ClassSessionModel{}).Where("id = ?", sess.ID).
			Update("booked_count", gorm.Expr("booked_count + 1")).Error; err != nil {
			return apperr.ErrInternalF("更新场次占用失败", err)
		}
		// 5. INSERT booking。
		actor := in.ActorID
		bk := BookingModel{
			BrandID:                in.BrandID,
			ClassSessionID:         sess.ID,
			BrandLearnerProfileID:  in.BrandLearnerProfileID,
			Source:                 string(booking.SourceStaffAssisted),
			Status:                 string(booking.StatusBooked),
			BookedAt:               now,
			AssistedBy:             &actor,
			RequiresEntitlementFix: requiresFix,
			NoEntitlementReason:    noReason,
		}
		if err := tx.Create(&bk).Error; err != nil {
			if be := bookingConflictError(err); be != nil {
				return be
			}
			return apperr.ErrInternalF("创建预约失败", err)
		}
		createdID = bk.ID

		// 6. 权益路径：扣额 + hold + 流水。
		if chosen != nil {
			countBased := chosen.TotalCredits != nil && chosen.RemainingCredits != nil
			delta := 0
			var balanceAfter *int
			upd := map[string]interface{}{"locked_credits": chosen.LockedCredits + 1}
			settleRemaining := chosen.RemainingCredits
			if countBased {
				nr := *chosen.RemainingCredits - 1
				if nr < 0 {
					return apperr.NewAppError(apperr.ErrEntitlementNotUsable, "权益余额不足", 422)
				}
				upd["remaining_credits"] = nr
				delta = -1
				balanceAfter = &nr
				settleRemaining = &nr
			}
			newStatus := entitlement.SettleStatus(entitlement.StatusActive, chosen.ExpiresAt, chosen.TotalCredits, settleRemaining, now)
			upd["status"] = string(newStatus)
			if err := tx.Model(&LearnerEntitlementModel{}).Where("id = ?", chosen.ID).Updates(upd).Error; err != nil {
				return apperr.ErrInternalF("锁定权益失败", err)
			}
			hold := EntitlementHoldModel{
				BrandID:               in.BrandID,
				BookingID:             bk.ID,
				LearnerEntitlementID:  chosen.ID,
				BrandLearnerProfileID: in.BrandLearnerProfileID,
				Credits:               1,
				Status:                string(booking.HoldStatusHeld),
				HeldAt:                now,
			}
			if err := tx.Create(&hold).Error; err != nil {
				if be := bookingConflictError(err); be != nil {
					return be
				}
				return apperr.ErrInternalF("锁定权益失败", err)
			}
			if err := insertBookingTransaction(tx, in.BrandID, chosen.ID, in.BrandLearnerProfileID, &bk.ID, &hold.ID, entitlement.ActionHold, delta, balanceAfter, "预约锁定", &actor); err != nil {
				return err
			}
			_ = chosenProd
		}

		// 7. audit。
		action := "booking_created"
		if requiresFix {
			action = "booking_created_no_entitlement"
		}
		return writeBookingLog(tx, in.BrandID, in.ActorID, action, bk.ID, nil, &bk)
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
		if err := r.applyCancel(tx, &bk, &sess, actorID, booking.CancelSourceStaff, reason, eff.ReleaseOnCancel, now); err != nil {
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

// applyCancel 把单条 booked 预约置 cancelled + 退名额 + 处理 hold（release / forfeit）。
// 调用方须已锁 session 行与 booking 行。release=false 时走 forfeit（hold→consumed，权益不回退）。
func (r *bookingRepository) applyCancel(tx *gorm.DB, bk *BookingModel, sess *ClassSessionModel, actorID int64, cancelSource, reason string, release bool, now time.Time) error {
	cs := cancelSource
	actor := actorID
	upd := map[string]interface{}{
		"status":        string(booking.StatusCancelled),
		"cancelled_at":  now,
		"cancelled_by":  actor,
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
	return settleHoldOnCancel(tx, bk.BrandID, bk.ID, bk.BrandLearnerProfileID, actorID, release, now)
}

// settleHoldOnCancel 处理取消时的 hold：release=true 释放权益（locked--/remaining++/re-settle），
// release=false 没收（hold→consumed，locked--/consumed++，remaining 不回退）。无 hold（占位预约）直接返回。
// 自由函数（同包）：TX-2 代取消与 TX-3 场次取消级联共用。
func settleHoldOnCancel(tx *gorm.DB, brandID, bookingID, learnerID, actorID int64, release bool, now time.Time) error {
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
	actor := actorID
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
		return insertBookingTransaction(tx, brandID, le.ID, learnerID, &bookingID, &hold.ID, entitlement.ActionRelease, delta, balanceAfter, "取消释放", &actor)
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
	return insertBookingTransaction(tx, brandID, le.ID, learnerID, &bookingID, &hold.ID, entitlement.ActionConsume, 0, le.RemainingCredits, "取消没收", &actor)
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
func insertBookingTransaction(tx *gorm.DB, brandID, entitlementID, learnerID int64, bookingID, holdID *int64, action entitlement.Action, delta int, balanceAfter *int, note string, operatedBy *int64) error {
	row := EntitlementTransactionModel{
		BrandID:               brandID,
		LearnerEntitlementID:  entitlementID,
		BrandLearnerProfileID: learnerID,
		BookingID:             bookingID,
		HoldID:                holdID,
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
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
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
