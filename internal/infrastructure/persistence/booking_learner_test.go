package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// TestLearnerBooking_Create 学员自助下单（Batch 14a）：source=learner_self_service + assisted_by NULL +
// auto 选权益 + hold + txn(operated_by NULL) + audit(actor=learner)。
func TestLearnerBooking_Create(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	ent := grantEnt(t, db, f.brandID, f.learner, "10次卡", "class_pack", intp(10), "all", "all", nil, nil)

	bk, err := repo.CreateByLearner(context.Background(), booking.LearnerCreateInput{
		BrandID: f.brandID, ClassSessionID: f.session, BrandLearnerProfileID: f.learner,
	})
	if err != nil {
		t.Fatalf("create by learner: %v", err)
	}
	if bk.Source != booking.SourceLearnerSelfService || bk.Status != booking.StatusBooked {
		t.Errorf("shape: source=%s status=%s", bk.Source, bk.Status)
	}
	if bk.AssistedBy != nil {
		t.Errorf("assisted_by should be NULL, got %v", *bk.AssistedBy)
	}
	if bk.Hold == nil || bk.Hold.Status != booking.HoldStatusHeld {
		t.Fatalf("hold missing/wrong: %+v", bk.Hold)
	}

	// psql: source + assisted_by NULL
	var source string
	var assisted *int64
	if err := db.Raw(`SELECT source, assisted_by FROM bookings WHERE id = ?`, bk.ID).Row().Scan(&source, &assisted); err != nil {
		t.Fatalf("scan booking: %v", err)
	}
	if source != "learner_self_service" || assisted != nil {
		t.Errorf("DB booking: source=%q assisted_by=%v, want learner_self_service/NULL", source, assisted)
	}
	// txn operated_by NULL（自助无 brand_user）
	var operatedBy *int64
	db.Raw(`SELECT operated_by FROM entitlement_transactions WHERE booking_id = ? AND action = 'hold'`, bk.ID).Row().Scan(&operatedBy)
	if operatedBy != nil {
		t.Errorf("txn operated_by should be NULL, got %v", *operatedBy)
	}
	// entitlement 扣额
	e := mustEntitlement(t, db, ent)
	if *e.RemainingCredits != 9 || e.LockedCredits != 1 {
		t.Errorf("entitlement remaining=%d locked=%d, want 9/1", *e.RemainingCredits, e.LockedCredits)
	}
	// audit actor=learner
	var actorType string
	var actorID int64
	db.Raw(`SELECT actor_type, actor_id FROM operation_logs WHERE action = 'booking_created' ORDER BY id DESC LIMIT 1`).Row().Scan(&actorType, &actorID)
	if actorType != "learner" || actorID != f.learner {
		t.Errorf("audit actor: type=%q id=%d, want learner/%d", actorType, actorID, f.learner)
	}
}

// TestLearnerBooking_NoEntitlement 无可用权益 → ENTITLEMENT_NONE_AVAILABLE（§7.3 引导联系机构）。
func TestLearnerBooking_NoEntitlement(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	_, err := repo.CreateByLearner(context.Background(), booking.LearnerCreateInput{
		BrandID: f.brandID, ClassSessionID: f.session, BrandLearnerProfileID: f.learner,
	})
	assertAppCode(t, err, apperr.ErrEntitlementNoneAvailable)
}

// TestLearnerBooking_TimeConflict §22.1 跨场次时间重叠 → BOOKING_TIME_CONFLICT；不重叠 → 成功。
func TestLearnerBooking_TimeConflict(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	// 不限次会员卡，排除频次/余额干扰，聚焦重叠维度。
	grantEnt(t, db, f.brandID, f.learner, "月卡", "membership_card", nil, "all", "all", nil, nil)

	base := time.Now().UTC().Add(48 * time.Hour)
	// 各场次用独立 instructor，避开 DB EXCLUDE（教练同时段重叠）。
	s1 := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), base, 8) // [base, base+1h]
	if _, err := repo.CreateByLearner(context.Background(), booking.LearnerCreateInput{BrandID: f.brandID, ClassSessionID: s1, BrandLearnerProfileID: f.learner}); err != nil {
		t.Fatalf("book s1: %v", err)
	}
	// 重叠：[base+30m, base+90m] 与 s1 相交。
	s2 := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), base.Add(30*time.Minute), 8)
	_, err := repo.CreateByLearner(context.Background(), booking.LearnerCreateInput{BrandID: f.brandID, ClassSessionID: s2, BrandLearnerProfileID: f.learner})
	assertAppCode(t, err, apperr.ErrBookingTimeConflict)
	// 不重叠：[base+2h, base+3h]。
	s3 := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), base.Add(2*time.Hour), 8)
	if _, err := repo.CreateByLearner(context.Background(), booking.LearnerCreateInput{BrandID: f.brandID, ClassSessionID: s3, BrandLearnerProfileID: f.learner}); err != nil {
		t.Fatalf("book s3 (no overlap) should succeed: %v", err)
	}
}
