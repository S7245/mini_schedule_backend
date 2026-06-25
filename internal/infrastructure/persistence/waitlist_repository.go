package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	domainbooking "github.com/zkw/mini-schedule/backend/internal/domain/booking"
	"github.com/zkw/mini-schedule/backend/internal/domain/learner"
	"github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type waitlistRepository struct {
	db *gorm.DB
	bk *bookingRepository // 复用 placeBooking / resolveEffectivePolicy（同包；只用 tx，不触 bk.db）
}

// NewWaitlistRepository 创建候补仓储。
func NewWaitlistRepository(db *gorm.DB) waitlist.Repository {
	return &waitlistRepository{db: db, bk: &bookingRepository{db: db}}
}

func waitlistConflictError(err error) error {
	name, ok := uniqueConstraint(err)
	if !ok {
		return nil
	}
	switch {
	case strings.Contains(name, "session_learner"):
		return apperr.NewAppError(apperr.ErrWaitlistDuplicate, "该学员已在该场次候补", 409)
	default:
		// position 冲突等（session 行锁下不该发生）退化为重复。
		return apperr.NewAppError(apperr.ErrWaitlistDuplicate, "候补已存在", 409)
	}
}

// ---- 反范式查询 ----

type waitlistRow struct {
	WaitlistEntryModel
	LearnerName     string    `gorm:"column:learner_name"`
	LearnerPhone    *string   `gorm:"column:learner_phone"`
	SessionStartsAt time.Time `gorm:"column:session_starts_at"`
	CourseTitle     string    `gorm:"column:course_title"`
	LocationID      int64     `gorm:"column:location_id"`
}

func (r *waitlistRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("waitlist_entries w").
		Select(`w.*,
			COALESCE(NULLIF(p.nickname, ''), li.nickname) AS learner_name, li.phone AS learner_phone,
			cs.starts_at AS session_starts_at, c.title AS course_title, cs.location_id`).
		Joins("JOIN class_sessions cs ON cs.id = w.class_session_id").
		Joins("JOIN courses c ON c.id = cs.course_id").
		Joins("JOIN brand_learner_profiles p ON p.id = w.brand_learner_profile_id").
		Joins("JOIN learner_identities li ON li.id = p.learner_identity_id")
}

func toWaitlistDomain(r *waitlistRow) *waitlist.Entry {
	e := &waitlist.Entry{
		ID:                    r.ID,
		BrandID:               r.BrandID,
		ClassSessionID:        r.ClassSessionID,
		BrandLearnerProfileID: r.BrandLearnerProfileID,
		Position:              r.Position,
		Status:                waitlist.Status(r.Status),
		PromotedBookingID:     r.PromotedBookingID,
		OperatedBy:            r.OperatedBy,
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
		LearnerName:           r.LearnerName,
		SessionStartsAt:       r.SessionStartsAt,
		CourseTitle:           r.CourseTitle,
		LocationID:            r.LocationID,
	}
	if r.SkippedReason != nil {
		e.SkippedReason = *r.SkippedReason
	}
	if r.LearnerPhone != nil {
		e.LearnerPhone = *r.LearnerPhone
	}
	return e
}

func (r *waitlistRepository) GetByID(ctx context.Context, brandID, id int64) (*waitlist.Entry, error) {
	var row waitlistRow
	if err := r.baseQuery(ctx).Where("w.id = ? AND w.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询候补失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrWaitlistEntryNotFound, "候补不存在", 404)
	}
	return toWaitlistDomain(&row), nil
}

func (r *waitlistRepository) ListBySession(ctx context.Context, brandID, sessionID int64, scopeLocationIDs []int64) ([]*waitlist.Entry, error) {
	// 越权场次按不存在处理（不泄漏存在性）。
	var sess ClassSessionModel
	if err := r.db.WithContext(ctx).Where("id = ? AND brand_id = ?", sessionID, brandID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询场次失败", err)
	}
	if scopeLocationIDs != nil && !int64InSlice(sess.LocationID, scopeLocationIDs) {
		return nil, apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
	}
	var rows []waitlistRow
	// 活跃(waiting/eligible)在前按 position，其余(promoted/cancelled/skipped)末尾。
	if err := r.baseQuery(ctx).Where("w.class_session_id = ? AND w.brand_id = ?", sessionID, brandID).
		Order("CASE WHEN w.status IN ('waiting','eligible_to_promote') THEN 0 ELSE 1 END, w.position ASC, w.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询候补名单失败", err)
	}
	items := make([]*waitlist.Entry, len(rows))
	for i := range rows {
		items[i] = toWaitlistDomain(&rows[i])
	}
	return items, nil
}

// ---- W1 加入候补 ----

func (r *waitlistRepository) Join(ctx context.Context, in waitlist.JoinInput) (*waitlist.Entry, error) {
	var createdID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", in.ClassSessionID, in.BrandID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
			}
			return apperr.ErrInternalF("查询场次失败", err)
		}
		if in.ScopeLocationIDs != nil && !int64InSlice(sess.LocationID, in.ScopeLocationIDs) {
			return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
		}
		if sess.Status != "scheduled" {
			return apperr.NewAppError(apperr.ErrSessionNotBookable, "场次当前不可预约", 409)
		}
		eff, perr := r.bk.resolveEffectivePolicy(tx, in.BrandID, sess.LocationID, sess.ID)
		if perr != nil {
			return perr
		}
		if !eff.AllowWaitlist {
			return apperr.NewAppError(apperr.ErrWaitlistNotAllowed, "该场次不允许候补", 409)
		}
		if sess.BookedCount < sess.Capacity {
			return apperr.NewAppError(apperr.ErrWaitlistSessionNotFull, "场次未满，请直接预约", 409)
		}
		// 学员可预约。
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
		// 已有该场次 active 预约 → 无需候补。
		var booked int64
		if err := tx.Model(&BookingModel{}).
			Where("class_session_id = ? AND brand_learner_profile_id = ? AND status <> 'cancelled'", sess.ID, in.BrandLearnerProfileID).
			Count(&booked).Error; err != nil {
			return apperr.ErrInternalF("查询预约失败", err)
		}
		if booked > 0 {
			return apperr.NewAppError(apperr.ErrBookingDuplicate, "该学员已预约该场次", 409)
		}
		// 已在候补（活跃）→ DUPLICATE（先于 limit 检查更友好；partial unique 留作并发兜底）。
		var dupWl int64
		if err := tx.Model(&WaitlistEntryModel{}).
			Where("class_session_id = ? AND brand_learner_profile_id = ? AND status IN ('waiting','eligible_to_promote')", sess.ID, in.BrandLearnerProfileID).
			Count(&dupWl).Error; err != nil {
			return apperr.ErrInternalF("查询候补失败", err)
		}
		if dupWl > 0 {
			return apperr.NewAppError(apperr.ErrWaitlistDuplicate, "该学员已在该场次候补", 409)
		}
		// waitlist_limit（0=不限）。
		var active int64
		if err := tx.Model(&WaitlistEntryModel{}).
			Where("class_session_id = ? AND status IN ('waiting','eligible_to_promote')", sess.ID).
			Count(&active).Error; err != nil {
			return apperr.ErrInternalF("统计候补失败", err)
		}
		if eff.WaitlistLimit > 0 && int(active) >= eff.WaitlistLimit {
			return apperr.NewAppError(apperr.ErrWaitlistFull, "候补名额已满", 409)
		}
		// position = 活跃 max + 1（session 行锁串行化）。
		var maxPos *int
		if err := tx.Model(&WaitlistEntryModel{}).
			Where("class_session_id = ? AND status IN ('waiting','eligible_to_promote')", sess.ID).
			Select("MAX(position)").Scan(&maxPos).Error; err != nil {
			return apperr.ErrInternalF("计算候补位置失败", err)
		}
		pos := 1
		if maxPos != nil {
			pos = *maxPos + 1
		}
		// 操作者：staff 代加入 operated_by=ActorID + audit brand_user；C 端学员自助 operated_by=NULL
		// （brand_users FK）+ audit actor=learner（actor_id=profileID）。
		var opBy *int64
		logActor := audit.Actor{Type: audit.ActorBrandUser, ID: in.ActorID}
		if in.SelfService {
			logActor = audit.Actor{Type: audit.ActorLearner, ID: in.BrandLearnerProfileID}
		} else {
			a := in.ActorID
			opBy = &a
		}
		entry := WaitlistEntryModel{
			BrandID:               in.BrandID,
			ClassSessionID:        sess.ID,
			BrandLearnerProfileID: in.BrandLearnerProfileID,
			Position:              pos,
			Status:                string(waitlist.StatusWaiting),
			OperatedBy:            opBy,
		}
		if err := tx.Create(&entry).Error; err != nil {
			if we := waitlistConflictError(err); we != nil {
				return we
			}
			return apperr.ErrInternalF("加入候补失败", err)
		}
		createdID = entry.ID
		return writeWaitlistLogAs(tx, logActor, in.BrandID, "waitlist_joined", entry.ID, nil, &entry)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, createdID)
}

// ---- W2 转正（手动）----

func (r *waitlistRepository) Promote(ctx context.Context, in waitlist.PromoteInput) (*waitlist.Entry, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pre WaitlistEntryModel
		if err := tx.Where("id = ? AND brand_id = ?", in.EntryID, in.BrandID).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrWaitlistEntryNotFound, "候补不存在", 404)
			}
			return apperr.ErrInternalF("查询候补失败", err)
		}
		// 锁序：先场次后候补（与下单/取消一致）。
		var sess ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", pre.ClassSessionID).First(&sess).Error; err != nil {
			return apperr.ErrInternalF("查询场次失败", err)
		}
		var entry WaitlistEntryModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", in.EntryID, in.BrandID).First(&entry).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrWaitlistEntryNotFound, "候补不存在", 404)
			}
			return apperr.ErrInternalF("查询候补失败", err)
		}
		if entry.Status != string(waitlist.StatusWaiting) {
			return apperr.NewAppError(apperr.ErrWaitlistNotPromotable, "该候补当前不可转正", 409)
		}
		if sess.Status != "scheduled" {
			return apperr.NewAppError(apperr.ErrSessionNotBookable, "场次当前不可预约", 409)
		}
		eff, perr := r.bk.resolveEffectivePolicy(tx, in.BrandID, sess.LocationID, sess.ID)
		if perr != nil {
			return perr
		}
		// 复用下单核心（容量/权益/booked++/booking/hold），source=waitlist_promotion。staff 转正：assisted_by = 员工。
		actor := in.ActorID
		bk, berr := r.bk.placeBooking(tx, &sess, eff, in.BrandID, &actor, entry.BrandLearnerProfileID,
			domainbooking.EntitlementMode(in.EntitlementMode), in.LearnerEntitlementID, in.NoEntitlementReason,
			string(domainbooking.SourceWaitlistPromotion), now)
		if berr != nil {
			return berr
		}
		if err := tx.Model(&WaitlistEntryModel{}).Where("id = ?", entry.ID).Updates(map[string]interface{}{
			"status":              string(waitlist.StatusPromoted),
			"promoted_booking_id": bk.ID,
			"operated_by":         actor,
		}).Error; err != nil {
			return apperr.ErrInternalF("更新候补状态失败", err)
		}
		action := "booking_created"
		if bk.RequiresEntitlementFix {
			action = "booking_created_no_entitlement"
		}
		if err := writeBookingLog(tx, in.BrandID, in.ActorID, action, bk.ID, nil, bk); err != nil {
			return err
		}
		after := entry
		after.Status = string(waitlist.StatusPromoted)
		after.PromotedBookingID = &bk.ID
		return writeWaitlistLog(tx, in.BrandID, in.ActorID, "waitlist_promoted", entry.ID, &entry, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, in.EntryID)
}

// ---- W3 跳过 / 取消 ----

func (r *waitlistRepository) Skip(ctx context.Context, brandID, actorID, id int64, reason string) (*waitlist.Entry, error) {
	if err := r.transition(ctx, brandID, actorID, id, []string{string(waitlist.StatusWaiting)},
		string(waitlist.StatusSkipped), strings.TrimSpace(reason), "waitlist_skipped"); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func (r *waitlistRepository) Cancel(ctx context.Context, brandID, actorID, id int64) (*waitlist.Entry, error) {
	if err := r.transition(ctx, brandID, actorID, id,
		[]string{string(waitlist.StatusWaiting), string(waitlist.StatusEligibleToPromote)},
		string(waitlist.StatusCancelled), "", "waitlist_cancelled"); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

// transition 锁 entry → 校 from 集合 → 置 to（+skipped_reason）+ audit。
func (r *waitlistRepository) transition(ctx context.Context, brandID, actorID, id int64, from []string, to, reason, action string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry WaitlistEntryModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", id, brandID).First(&entry).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrWaitlistEntryNotFound, "候补不存在", 404)
			}
			return apperr.ErrInternalF("查询候补失败", err)
		}
		ok := false
		for _, f := range from {
			if entry.Status == f {
				ok = true
				break
			}
		}
		if !ok {
			return apperr.NewAppError(apperr.ErrWaitlistNotPromotable, "该候补当前不可操作", 409)
		}
		actor := actorID
		upd := map[string]interface{}{"status": to, "operated_by": actor}
		if to == string(waitlist.StatusSkipped) {
			upd["skipped_reason"] = reason
		}
		if err := tx.Model(&WaitlistEntryModel{}).Where("id = ?", id).Updates(upd).Error; err != nil {
			return apperr.ErrInternalF("更新候补状态失败", err)
		}
		after := entry
		after.Status = to
		return writeWaitlistLog(tx, brandID, actorID, action, id, &entry, &after)
	})
}

func writeWaitlistLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *WaitlistEntryModel) error {
	return writeWaitlistLogAs(tx, audit.Actor{Type: audit.ActorBrandUser, ID: actorID}, brandID, action, id, before, after)
}

// writeWaitlistLogAs 同 writeWaitlistLog 但 actor 可指定（C 端学员自助传 {ActorLearner, profileID}，Batch 14b）。
func writeWaitlistLogAs(tx *gorm.DB, actor audit.Actor, brandID int64, action string, id int64, before, after *WaitlistEntryModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   actor,
		Action:  action,
		Target:  audit.Target{Type: "waitlist_entry", ID: id},
		Before:  before,
		After:   after,
	})
}
