package persistence

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
	"github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// fullSession 造一个 cap=1 的 scheduled 场次并用 L1 占满（带次卡），返回 (sessionID, L1 booking)。
func fullSession(t *testing.T, db *gorm.DB, f bkFixture, instr int64) (int64, *booking.Booking) {
	t.Helper()
	sess := seedScheduledSession(t, db, f.brandID, f.loc, f.course, instr, time.Now().UTC().Add(48*time.Hour), 1)
	grantEnt(t, db, f.brandID, f.learner, "L1卡", "class_pack", intp(10), "all", "all", nil, nil)
	bkRepo := NewBookingRepository(db)
	bk, err := bkRepo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	if err != nil {
		t.Fatalf("fill session: %v", err)
	}
	return sess, bk
}

func waitlistStatusOf(t *testing.T, db *gorm.DB, id int64) (string, *int64) {
	t.Helper()
	var e WaitlistEntryModel
	db.Where("id = ?", id).First(&e)
	return e.Status, e.PromotedBookingID
}

func TestWaitlist_JoinAndList(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, _ := fullSession(t, db, f, instr2)
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:l2")
	l3 := seedLearnerProfile(t, db, f.brandID, "wl:l3")

	e2, err := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})
	if err != nil {
		t.Fatalf("join l2: %v", err)
	}
	if e2.Position != 1 || e2.Status != waitlist.StatusWaiting {
		t.Errorf("l2 position=%d status=%s, want 1/waiting", e2.Position, e2.Status)
	}
	e3, err := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l3})
	if err != nil {
		t.Fatalf("join l3: %v", err)
	}
	if e3.Position != 2 {
		t.Errorf("l3 position=%d, want 2", e3.Position)
	}
	list, err := repo.ListBySession(context.Background(), f.brandID, sess, nil)
	if err != nil || len(list) != 2 {
		t.Fatalf("list len=%d err=%v, want 2", len(list), err)
	}
	if list[0].Position != 1 || list[1].Position != 2 {
		t.Errorf("list order wrong: %d,%d", list[0].Position, list[1].Position)
	}
}

func TestWaitlist_JoinEdges(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	bkRepo := NewBookingRepository(db)
	f := bookingSetup(t, db)

	// 未满（cap=8 空场次）→ SESSION_NOT_FULL。
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:l2")
	_, err := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: l2})
	assertAppCode(t, err, apperr.ErrWaitlistSessionNotFull)

	instr2 := seedInstructor(t, db, f.brandID)
	sess, bk1 := fullSession(t, db, f, instr2)

	// allow_waitlist=false → NOT_ALLOWED。
	if _, err := bkRepo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{AllowWaitlist: false, ReleaseOnCancel: true}); err != nil {
		t.Fatalf("policy: %v", err)
	}
	_, err = repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})
	assertAppCode(t, err, apperr.ErrWaitlistNotAllowed)

	// 开 waitlist + limit=1。
	if _, err := bkRepo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{AllowWaitlist: true, WaitlistLimit: 1, ReleaseOnCancel: true}); err != nil {
		t.Fatalf("policy2: %v", err)
	}
	if _, err := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2}); err != nil {
		t.Fatalf("join l2: %v", err)
	}
	// 重复候补 → DUPLICATE。
	_, err = repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})
	assertAppCode(t, err, apperr.ErrWaitlistDuplicate)
	// 超 limit（已 1 个活跃）→ FULL。
	l3 := seedLearnerProfile(t, db, f.brandID, "wl:l3")
	_, err = repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l3})
	assertAppCode(t, err, apperr.ErrWaitlistFull)
	// 已 active 预约的 L1 候补 → BOOKING_DUPLICATE。
	_, err = repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: f.learner})
	assertAppCode(t, err, apperr.ErrBookingDuplicate)
	_ = bk1
}

func TestWaitlist_PromoteFreesSlot(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	bkRepo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, bk1 := fullSession(t, db, f, instr2)
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:l2")
	ent2 := grantEnt(t, db, f.brandID, l2, "L2卡", "class_pack", intp(5), "all", "all", nil, nil)

	entry, _ := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})

	// 满员转正 → SESSION_FULL。
	_, err := repo.Promote(context.Background(), waitlist.PromoteInput{BrandID: f.brandID, ActorID: f.actor, EntryID: entry.ID, EntitlementMode: "auto"})
	assertAppCode(t, err, apperr.ErrSessionFull)

	// 取消 L1 腾位。
	if _, err := bkRepo.Cancel(context.Background(), f.brandID, f.actor, bk1.ID, "腾位"); err != nil {
		t.Fatalf("cancel l1: %v", err)
	}
	// 转正 L2（auto 选权益）。
	got, err := repo.Promote(context.Background(), waitlist.PromoteInput{BrandID: f.brandID, ActorID: f.actor, EntryID: entry.ID, EntitlementMode: "auto"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got.Status != waitlist.StatusPromoted || got.PromotedBookingID == nil {
		t.Fatalf("entry not promoted: %+v", got)
	}
	// psql 真值：booking source=waitlist_promotion、booked_count 回填、权益扣额。
	var bk BookingModel
	db.Where("id = ?", *got.PromotedBookingID).First(&bk)
	if bk.Source != string(booking.SourceWaitlistPromotion) || bk.Status != "booked" {
		t.Errorf("promoted booking source=%s status=%s", bk.Source, bk.Status)
	}
	if got := sessionBookedCount(t, db, sess); got != 1 {
		t.Errorf("booked_count=%d, want 1", got)
	}
	e := mustEntitlement(t, db, ent2)
	if *e.RemainingCredits != 4 || e.LockedCredits != 1 {
		t.Errorf("l2 entitlement remaining=%d locked=%d, want 4/1", *e.RemainingCredits, e.LockedCredits)
	}
	// 已 promoted 再转正 → NOT_PROMOTABLE。
	_, err = repo.Promote(context.Background(), waitlist.PromoteInput{BrandID: f.brandID, ActorID: f.actor, EntryID: entry.ID, EntitlementMode: "auto"})
	assertAppCode(t, err, apperr.ErrWaitlistNotPromotable)
}

func TestWaitlist_SkipCancelRejoin(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, _ := fullSession(t, db, f, instr2)
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:l2")

	e2, _ := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})
	// skip。
	if _, err := repo.Skip(context.Background(), f.brandID, f.actor, e2.ID, "无权益"); err != nil {
		t.Fatalf("skip: %v", err)
	}
	st, _ := waitlistStatusOf(t, db, e2.ID)
	if st != string(waitlist.StatusSkipped) {
		t.Errorf("status=%s, want skipped", st)
	}
	var sm WaitlistEntryModel
	db.Where("id = ?", e2.ID).First(&sm)
	if sm.SkippedReason == nil || *sm.SkippedReason != "无权益" {
		t.Errorf("skipped_reason=%v", sm.SkippedReason)
	}
	// skipped 后可重新候补（partial unique 放行新行）。
	e2b, err := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})
	if err != nil {
		t.Fatalf("rejoin after skip: %v", err)
	}
	if e2b.ID == e2.ID {
		t.Error("重新候补应是新行")
	}
	// cancel。
	if _, err := repo.Cancel(context.Background(), f.brandID, f.actor, e2b.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	st, _ = waitlistStatusOf(t, db, e2b.ID)
	if st != string(waitlist.StatusCancelled) {
		t.Errorf("status=%s, want cancelled", st)
	}
}

func TestSessionCancel_CascadesWaitlist(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	sessRepo := NewClassSessionRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, _ := fullSession(t, db, f, instr2) // L1 booked 占满 cap=1
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:l2")
	l3 := seedLearnerProfile(t, db, f.brandID, "wl:l3")
	e2, _ := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2})
	e3, _ := repo.Join(context.Background(), waitlist.JoinInput{BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l3})

	if _, err := sessRepo.Cancel(context.Background(), f.brandID, f.actor, sess, "场地维护"); err != nil {
		t.Fatalf("cancel session: %v", err)
	}
	// 活跃候补全 cancelled。
	for _, id := range []int64{e2.ID, e3.ID} {
		st, _ := waitlistStatusOf(t, db, id)
		if st != string(waitlist.StatusCancelled) {
			t.Errorf("waitlist %d status=%s, want cancelled", id, st)
		}
	}
	// L1 的 booking 也被 13c 级联取消。
	var bookedLeft int64
	db.Model(&BookingModel{}).Where("class_session_id = ? AND status = 'booked'", sess).Count(&bookedLeft)
	if bookedLeft != 0 {
		t.Errorf("场次取消后仍有 %d 个 booked", bookedLeft)
	}
}
