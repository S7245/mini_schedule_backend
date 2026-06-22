package persistence

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// ---- fixtures ----

func seedScheduledSession(t *testing.T, db *gorm.DB, brandID, loc, course, instr int64, startsAt time.Time, capacity int) int64 {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO class_sessions (brand_id, location_id, course_id, instructor_profile_id, starts_at, ends_at, capacity, booked_count, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 'scheduled')`,
		brandID, loc, course, instr, startsAt, startsAt.Add(time.Hour), capacity,
	).Error; err != nil {
		t.Fatalf("seed session: %v", err)
	}
	var id int64
	db.Raw(`SELECT id FROM class_sessions WHERE brand_id = ? ORDER BY id DESC LIMIT 1`, brandID).Scan(&id)
	return id
}

// grantEnt 造产品 + 发放权益。total=nil 表示会员卡（不限次）。返回 entitlementID。
func grantEnt(t *testing.T, db *gorm.DB, brandID, learnerID int64, name, ptype string, total *int, locScope, courseScope string, locIDs, courseIDs []int64) int64 {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO entitlement_products (brand_id, name, product_type, total_credits, validity_days, location_scope, course_scope, status)
		 VALUES (?, ?, ?, ?, 365, ?, ?, 'active')`,
		brandID, name, ptype, total, locScope, courseScope,
	).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	var pid int64
	db.Raw(`SELECT id FROM entitlement_products WHERE brand_id = ? AND name = ?`, brandID, name).Scan(&pid)
	for _, l := range locIDs {
		db.Exec(`INSERT INTO entitlement_product_locations (brand_id, product_id, location_id) VALUES (?, ?, ?)`, brandID, pid, l)
	}
	for _, c := range courseIDs {
		db.Exec(`INSERT INTO entitlement_product_courses (brand_id, product_id, course_id) VALUES (?, ?, ?)`, brandID, pid, c)
	}
	if err := db.Exec(
		`INSERT INTO learner_entitlements (brand_id, brand_learner_profile_id, product_id, status, total_credits, remaining_credits, locked_credits, consumed_credits, starts_at, expires_at)
		 VALUES (?, ?, ?, 'active', ?, ?, 0, 0, NOW(), NOW() + INTERVAL '300 days')`,
		brandID, learnerID, pid, total, total,
	).Error; err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
	var eid int64
	db.Raw(`SELECT id FROM learner_entitlements WHERE brand_id = ? AND product_id = ? ORDER BY id DESC LIMIT 1`, brandID, pid).Scan(&eid)
	return eid
}

type bkFixture struct {
	brandID, actor, loc, course, instr, session, learner int64
}

func bookingSetup(t *testing.T, db *gorm.DB) bkFixture {
	t.Helper()
	brandID, _ := seedBrandWithSystemRoles(t, db)
	actor := seedBrandUser(t, db, brandID)
	loc := seedLocation(t, db, brandID, "讯美广场")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "晨间瑜伽")
	session := seedScheduledSession(t, db, brandID, loc, course, instr, time.Now().UTC().Add(48*time.Hour), 8)
	learner := seedLearnerProfile(t, db, brandID, "bk:learner")
	return bkFixture{brandID, actor, loc, course, instr, session, learner}
}

func intp(v int) *int { return &v }

func mustEntitlement(t *testing.T, db *gorm.DB, id int64) LearnerEntitlementModel {
	t.Helper()
	var e LearnerEntitlementModel
	if err := db.Where("id = ?", id).First(&e).Error; err != nil {
		t.Fatalf("read entitlement %d: %v", id, err)
	}
	return e
}

func sessionBookedCount(t *testing.T, db *gorm.DB, id int64) int {
	t.Helper()
	var n int
	db.Raw(`SELECT booked_count FROM class_sessions WHERE id = ?`, id).Scan(&n)
	return n
}

// ---- TX-1 下单 ----

func TestBooking_CreateAuto_LocksCredit(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	ent := grantEnt(t, db, f.brandID, f.learner, "10次卡", "class_pack", intp(10), "all", "all", nil, nil)

	bk, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session,
		BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if bk.Status != booking.StatusBooked || bk.Source != booking.SourceStaffAssisted {
		t.Errorf("booking shape: %s/%s", bk.Status, bk.Source)
	}
	if bk.Hold == nil || bk.Hold.Status != booking.HoldStatusHeld {
		t.Fatalf("hold missing/wrong: %+v", bk.Hold)
	}
	if bk.CourseTitle != "晨间瑜伽" || bk.LocationName != "讯美广场" {
		t.Errorf("denormalized missing: course=%q location=%q", bk.CourseTitle, bk.LocationName)
	}
	if got := sessionBookedCount(t, db, f.session); got != 1 {
		t.Errorf("booked_count = %d, want 1", got)
	}
	e := mustEntitlement(t, db, ent)
	if *e.RemainingCredits != 9 || e.LockedCredits != 1 {
		t.Errorf("entitlement remaining=%d locked=%d, want 9/1", *e.RemainingCredits, e.LockedCredits)
	}
	var tx EntitlementTransactionModel
	db.Where("learner_entitlement_id = ? AND action = 'hold'", ent).First(&tx)
	if tx.DeltaCredits != -1 || tx.BookingID == nil || tx.HoldID == nil {
		t.Errorf("hold txn wrong: delta=%d booking=%v hold=%v", tx.DeltaCredits, tx.BookingID, tx.HoldID)
	}
}

func TestBooking_CreateManual_Membership_Unlimited(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	ent := grantEnt(t, db, f.brandID, f.learner, "月卡", "membership_card", nil, "all", "all", nil, nil)

	bk, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session,
		BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeManual, LearnerEntitlementID: &ent,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if bk.Hold == nil {
		t.Fatal("membership 也应建 hold")
	}
	e := mustEntitlement(t, db, ent)
	if e.RemainingCredits != nil {
		t.Errorf("不限次 remaining 应保持 NULL, got %v", *e.RemainingCredits)
	}
	if e.LockedCredits != 1 {
		t.Errorf("locked = %d, want 1", e.LockedCredits)
	}
	var tx EntitlementTransactionModel
	db.Where("learner_entitlement_id = ? AND action = 'hold'", ent).First(&tx)
	if tx.DeltaCredits != 0 || tx.BalanceAfter != nil {
		t.Errorf("不限次 hold txn 应 delta=0 balance=NULL, got delta=%d balance=%v", tx.DeltaCredits, tx.BalanceAfter)
	}
}

func TestBooking_NonePlaceholder(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)

	// 缺原因 → 422。
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session,
		BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeNone,
	})
	assertAppCode(t, err, apperr.ErrAssistedReasonRequired)

	bk, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session,
		BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeNone, NoEntitlementReason: "待补卡",
	})
	if err != nil {
		t.Fatalf("create placeholder: %v", err)
	}
	if !bk.RequiresEntitlementFix || bk.Hold != nil {
		t.Errorf("placeholder: fix=%v hold=%v", bk.RequiresEntitlementFix, bk.Hold)
	}
	if got := sessionBookedCount(t, db, f.session); got != 1 {
		t.Errorf("placeholder 也占容量, booked_count=%d", got)
	}
}

func TestBooking_SessionFull(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	// capacity=1 的场次。
	full := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), time.Now().UTC().Add(72*time.Hour), 1)
	l2 := seedLearnerProfile(t, db, f.brandID, "bk:l2")
	grantEnt(t, db, f.brandID, f.learner, "卡A", "class_pack", intp(5), "all", "all", nil, nil)

	if _, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: full, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	}); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	// 第二个学员占位也不能绕容量。
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: full, BrandLearnerProfileID: l2, EntitlementMode: booking.ModeNone, NoEntitlementReason: "x",
	})
	assertAppCode(t, err, apperr.ErrSessionFull)
}

func TestBooking_DuplicateThenRebookAfterCancel(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	grantEnt(t, db, f.brandID, f.learner, "卡", "class_pack", intp(5), "all", "all", nil, nil)

	bk1, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// 重复预约 → BOOKING_DUPLICATE。
	_, err = repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	assertAppCode(t, err, apperr.ErrBookingDuplicate)

	// 取消后可重约（partial unique 放行新行）。
	if _, err := repo.Cancel(context.Background(), f.brandID, f.actor, bk1.ID, "改期"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	bk2, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	if err != nil {
		t.Fatalf("rebook after cancel: %v", err)
	}
	if bk2.ID == bk1.ID {
		t.Error("重约应是新行")
	}
}

func TestBooking_Cancel_ReleasesHold(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	ent := grantEnt(t, db, f.brandID, f.learner, "卡", "class_pack", intp(10), "all", "all", nil, nil)

	bk, _ := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	if _, err := repo.Cancel(context.Background(), f.brandID, f.actor, bk.ID, "取消"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got := sessionBookedCount(t, db, f.session); got != 0 {
		t.Errorf("取消后 booked_count = %d, want 0", got)
	}
	e := mustEntitlement(t, db, ent)
	if *e.RemainingCredits != 10 || e.LockedCredits != 0 {
		t.Errorf("取消后 remaining=%d locked=%d, want 10/0", *e.RemainingCredits, e.LockedCredits)
	}
	var h EntitlementHoldModel
	db.Where("booking_id = ?", bk.ID).First(&h)
	if h.Status != string(booking.HoldStatusReleased) {
		t.Errorf("hold status = %s, want released", h.Status)
	}
	// 不可取消已取消的。
	_, err := repo.Cancel(context.Background(), f.brandID, f.actor, bk.ID, "again")
	assertAppCode(t, err, apperr.ErrBookingNotCancellable)
}

func TestBooking_Cancel_ForfeitWhenNoRelease(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	ent := grantEnt(t, db, f.brandID, f.learner, "卡", "class_pack", intp(10), "all", "all", nil, nil)
	// 品牌默认 release_on_cancel=false。
	if _, err := repo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{ReleaseOnCancel: false, AllowWaitlist: true}); err != nil {
		t.Fatalf("policy: %v", err)
	}
	bk, _ := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	if _, err := repo.Cancel(context.Background(), f.brandID, f.actor, bk.ID, "no-release"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	e := mustEntitlement(t, db, ent)
	if *e.RemainingCredits != 9 || e.LockedCredits != 0 || e.ConsumedCredits != 1 {
		t.Errorf("forfeit: remaining=%d locked=%d consumed=%d, want 9/0/1", *e.RemainingCredits, e.LockedCredits, e.ConsumedCredits)
	}
	var h EntitlementHoldModel
	db.Where("booking_id = ?", bk.ID).First(&h)
	if h.Status != string(booking.HoldStatusConsumed) {
		t.Errorf("forfeit hold status = %s, want consumed", h.Status)
	}
}

func TestBooking_AutoNoneAvailable(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	assertAppCode(t, err, apperr.ErrEntitlementNoneAvailable)
}

func TestBooking_ManualScopeMismatch(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	otherCourse := seedPublishedCourseAt(t, db, f.brandID, f.loc, "其他课")
	// 权益限定 otherCourse，与 session 的 course 不符。
	ent := grantEnt(t, db, f.brandID, f.learner, "指定课卡", "class_pack", intp(5), "all", "specific", nil, []int64{otherCourse})
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner,
		EntitlementMode: booking.ModeManual, LearnerEntitlementID: &ent,
	})
	assertAppCode(t, err, apperr.ErrEntitlementScopeMismatch)
}

func TestBooking_FrequencyDailyExceeded(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	grantEnt(t, db, f.brandID, f.learner, "卡", "class_pack", intp(10), "all", "all", nil, nil)
	// 每日上限 1。
	if _, err := repo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{DailyBookingLimit: intp(1), ReleaseOnCancel: true, AllowWaitlist: true}); err != nil {
		t.Fatalf("policy: %v", err)
	}
	// 同一天第二个场次（不同时间，避免教练 EXCLUDE）。
	day := f.session // 第一个 session starts +48h
	_ = day
	base := time.Now().UTC().Add(48 * time.Hour)
	instr2 := seedInstructor(t, db, f.brandID)
	sess2 := seedScheduledSession(t, db, f.brandID, f.loc, f.course, instr2, base.Add(3*time.Hour), 8)

	if _, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	}); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess2, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	assertAppCode(t, err, apperr.ErrBookingFrequencyExceeded)
}

func TestBooking_WindowClosed(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	grantEnt(t, db, f.brandID, f.learner, "卡", "class_pack", intp(5), "all", "all", nil, nil)
	// 最少提前 7 天，但 session 仅 +48h → 窗口已关。
	if _, err := repo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{BookAheadMinMinutes: 7 * 24 * 60, ReleaseOnCancel: true, AllowWaitlist: true}); err != nil {
		t.Fatalf("policy: %v", err)
	}
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	assertAppCode(t, err, apperr.ErrBookingWindowClosed)
}

func TestBooking_DataScopeOutOfScope(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	grantEnt(t, db, f.brandID, f.learner, "卡", "class_pack", intp(5), "all", "all", nil, nil)
	// scope 只含 loc+999（不含 session 所在 loc）→ 越权按不存在。
	_, err := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner,
		EntitlementMode: booking.ModeAuto, ScopeLocationIDs: []int64{f.loc + 999},
	})
	assertAppCode(t, err, apperr.ErrSessionNotFound)
}

func TestBooking_PolicyUpsertAndGet(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	// 无行返默认。
	def, err := repo.GetDefaultPolicy(context.Background(), f.brandID)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if !def.ReleaseOnCancel || def.BookAheadMaxMinutes != nil {
		t.Errorf("default policy wrong: %+v", def)
	}
	if _, err := repo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{
		BookAheadMaxMinutes: intp(10080), CancelDeadlineMinutes: 120, ReleaseOnCancel: true, DailyBookingLimit: intp(2), AllowWaitlist: true,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, _ := repo.GetDefaultPolicy(context.Background(), f.brandID)
	if got.CancelDeadlineMinutes != 120 || got.BookAheadMaxMinutes == nil || *got.BookAheadMaxMinutes != 10080 || got.DailyBookingLimit == nil || *got.DailyBookingLimit != 2 {
		t.Errorf("upserted policy wrong: %+v", got)
	}
	// 二次 upsert 覆盖。
	if _, err := repo.UpsertDefaultPolicy(context.Background(), f.brandID, f.actor, booking.Policy{CancelDeadlineMinutes: 30, ReleaseOnCancel: false, AllowWaitlist: false}); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	got, _ = repo.GetDefaultPolicy(context.Background(), f.brandID)
	if got.CancelDeadlineMinutes != 30 || got.ReleaseOnCancel {
		t.Errorf("second upsert not applied: %+v", got)
	}
}
