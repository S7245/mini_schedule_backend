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

// setSessionTimesStatus 直接改场次时间/状态，造扫描场景。
func setSessionTimesStatus(t *testing.T, db *gorm.DB, id int64, startsAt, endsAt time.Time, status string) {
	t.Helper()
	if err := db.Exec(`UPDATE class_sessions SET starts_at = ?, ends_at = ?, status = ? WHERE id = ?`,
		startsAt, endsAt, status, id).Error; err != nil {
		t.Fatalf("set session %d times/status: %v", id, err)
	}
}

func sessionStatusOf(t *testing.T, db *gorm.DB, id int64) string {
	t.Helper()
	var s string
	db.Raw(`SELECT status FROM class_sessions WHERE id = ?`, id).Scan(&s)
	return s
}

func countSystemEndAudit(t *testing.T, db *gorm.DB, sessionID int64) int {
	t.Helper()
	var n int
	db.Raw(`SELECT COUNT(*) FROM operation_logs
	        WHERE action = 'session_ended' AND target_id = ?
	          AND actor_type = 'system' AND actor_id IS NULL`, sessionID).Scan(&n)
	return n
}

// ---- EndSessionSystem ----

func TestEndSessionSystem_SweepsAndAuditsSystem(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	l2 := seedLearnerProfile(t, db, f.brandID, "sys:l2")
	grantEnt(t, db, f.brandID, f.learner, "卡1", "class_pack", intp(10), "all", "all", nil, nil)
	grantEnt(t, db, f.brandID, l2, "卡2", "class_pack", intp(10), "all", "all", nil, nil)
	bk1, _ := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: f.learner, EntitlementMode: booking.ModeAuto,
	})
	bk2, _ := repo.Create(context.Background(), booking.CreateInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: f.session, BrandLearnerProfileID: l2, EntitlementMode: booking.ModeAuto,
	})
	// 签到 bk1，留 bk2 未签到。
	if _, err := repo.Attend(context.Background(), f.brandID, f.actor, bk1.ID, ""); err != nil {
		t.Fatalf("attend bk1: %v", err)
	}
	bookedBefore := sessionBookedCount(t, db, f.session)

	res, err := repo.EndSessionSystem(context.Background(), f.session)
	if err != nil {
		t.Fatalf("EndSessionSystem: %v", err)
	}
	if res.Status != "completed" || res.PendingNoShowCount != 1 {
		t.Errorf("result status=%s pending=%d, want completed/1", res.Status, res.PendingNoShowCount)
	}
	if got := sessionStatusOf(t, db, f.session); got != "completed" {
		t.Errorf("session status = %s, want completed", got)
	}
	if s1, _ := bookingStatusOf(t, db, bk1.ID); s1 != string(booking.StatusAttended) {
		t.Errorf("bk1 = %s, want attended (结束不动已签到)", s1)
	}
	if s2, _ := bookingStatusOf(t, db, bk2.ID); s2 != string(booking.StatusPendingNoShow) {
		t.Errorf("bk2 = %s, want pending_no_show", s2)
	}
	if got := sessionBookedCount(t, db, f.session); got != bookedBefore {
		t.Errorf("booked_count = %d, want unchanged %d (结束不退名额)", got, bookedBefore)
	}
	// §22.6：不自动 no_show。
	if s2, _ := bookingStatusOf(t, db, bk2.ID); s2 == string(booking.StatusNoShow) {
		t.Error("§22.6 违反：自动 EndSession 不得把 booking 推到 no_show")
	}
	// audit：actor_type=system + actor_id NULL。
	if n := countSystemEndAudit(t, db, f.session); n != 1 {
		t.Errorf("system session_ended audit count = %d, want 1 (actor_type=system, actor_id NULL)", n)
	}
}

func TestEndSessionSystem_Idempotent(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	if _, err := repo.EndSessionSystem(context.Background(), f.session); err != nil {
		t.Fatalf("first EndSessionSystem: %v", err)
	}
	// 第二遍：completed 场次不可再结束 → SESSION_NOT_ENDABLE（幂等：空操作不重复扣/不重复 audit）。
	_, err := repo.EndSessionSystem(context.Background(), f.session)
	assertAppCode(t, err, apperr.ErrSessionNotEndable)
	if n := countSystemEndAudit(t, db, f.session); n != 1 {
		t.Errorf("audit count after重复 = %d, want still 1", n)
	}
}

// TestWaitlist_PromoteIntoInProgress Batch 15 回归（code-review F1）：场次自动转 in_progress 后，
// 已候补学员仍可转正（in_progress 视同 scheduled）。修复前 waitlist Promote 的 `status != scheduled`
// 守卫会在场次到点自动转 in_progress 后阻断「课已开始、有人腾位、把候补者转正」这一真实工作流。
func TestWaitlist_PromoteIntoInProgress(t *testing.T) {
	db := newMigratedTestDB(t)
	wlRepo := NewWaitlistRepository(db)
	bkRepo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, bk1 := fullSession(t, db, f, instr2)
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:ip")
	grantEnt(t, db, f.brandID, l2, "卡", "class_pack", intp(5), "all", "all", nil, nil)
	entry, err := wlRepo.Join(context.Background(), waitlist.JoinInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2,
	})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	// 腾位（场次此时仍 scheduled）。
	if _, err := bkRepo.Cancel(context.Background(), f.brandID, f.actor, bk1.ID, "腾位"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// 场次到点自动转 in_progress（starts 过去、ends 未来）。
	now := time.Now().UTC()
	setSessionTimesStatus(t, db, sess, now.Add(-10*time.Minute), now.Add(50*time.Minute), "in_progress")
	// 转正应成功（修复前会被 SESSION_NOT_BOOKABLE 阻断）。
	got, err := wlRepo.Promote(context.Background(), waitlist.PromoteInput{
		BrandID: f.brandID, ActorID: f.actor, EntryID: entry.ID, EntitlementMode: "auto",
	})
	if err != nil {
		t.Fatalf("promote into in_progress should succeed (Batch 15 视同 scheduled): %v", err)
	}
	if got.Status != waitlist.StatusPromoted || got.PromotedBookingID == nil {
		t.Fatalf("entry not promoted: %+v", got)
	}
}

func TestEndSessionSystem_InProgressToCompleted(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	now := time.Now().UTC()
	setSessionTimesStatus(t, db, f.session, now.Add(-2*time.Hour), now.Add(-time.Hour), "in_progress")
	res, err := repo.EndSessionSystem(context.Background(), f.session)
	if err != nil {
		t.Fatalf("EndSessionSystem(in_progress): %v", err)
	}
	if res.Status != "completed" {
		t.Errorf("status = %s, want completed", res.Status)
	}
}

// ---- MarkSessionsInProgress ----

func TestMarkSessionsInProgress(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)
	now := time.Now().UTC()

	// 各场次独立教练，避开 instructor_no_overlap EXCLUDE（本测试不关心排课冲突）。
	// A: scheduled + 进行中窗口（starts 过去, ends 未来）→ 应转 in_progress。
	a := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), now.Add(-30*time.Minute), 8)
	// B: scheduled + 未来 → 不动。
	b := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), now.Add(2*time.Hour), 8)
	// C: completed → 不动。
	c := seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), now.Add(-30*time.Minute), 8)
	setSessionTimesStatus(t, db, c, now.Add(-30*time.Minute), now.Add(30*time.Minute), "completed")

	n, err := repo.MarkSessionsInProgress(context.Background(), now)
	if err != nil {
		t.Fatalf("MarkSessionsInProgress: %v", err)
	}
	if n != 1 {
		t.Errorf("marked = %d, want 1 (仅 A)", n)
	}
	if got := sessionStatusOf(t, db, a); got != "in_progress" {
		t.Errorf("A = %s, want in_progress", got)
	}
	if got := sessionStatusOf(t, db, b); got != "scheduled" {
		t.Errorf("B = %s, want scheduled (未到点)", got)
	}
	if got := sessionStatusOf(t, db, c); got != "completed" {
		t.Errorf("C = %s, want completed (不回退)", got)
	}
	// 幂等：再跑一轮 0 行。
	n2, _ := repo.MarkSessionsInProgress(context.Background(), now)
	if n2 != 0 {
		t.Errorf("second mark = %d, want 0 (幂等)", n2)
	}
}

// ---- ListDueSessionIDs ----

func TestListDueSessionIDs(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	now := time.Now().UTC()

	f1 := bookingSetup(t, db)
	// 各场次独立教练，避开 instructor_no_overlap EXCLUDE。
	// brand1 到点 scheduled（命中）。
	due1 := seedScheduledSession(t, db, f1.brandID, f1.loc, f1.course, seedInstructor(t, db, f1.brandID), now.Add(-2*time.Hour), 8)
	// brand1 到点 in_progress（命中）。
	dueIP := seedScheduledSession(t, db, f1.brandID, f1.loc, f1.course, seedInstructor(t, db, f1.brandID), now.Add(-2*time.Hour), 8)
	setSessionTimesStatus(t, db, dueIP, now.Add(-2*time.Hour), now.Add(-time.Hour), "in_progress")
	// brand1 未来 scheduled（不命中）。
	future := seedScheduledSession(t, db, f1.brandID, f1.loc, f1.course, seedInstructor(t, db, f1.brandID), now.Add(2*time.Hour), 8)
	// brand1 已完成（不命中）。
	done := seedScheduledSession(t, db, f1.brandID, f1.loc, f1.course, seedInstructor(t, db, f1.brandID), now.Add(-2*time.Hour), 8)
	setSessionTimesStatus(t, db, done, now.Add(-2*time.Hour), now.Add(-time.Hour), "completed")
	// brand1 已取消（不命中）。
	cancelled := seedScheduledSession(t, db, f1.brandID, f1.loc, f1.course, seedInstructor(t, db, f1.brandID), now.Add(-2*time.Hour), 8)
	setSessionTimesStatus(t, db, cancelled, now.Add(-2*time.Hour), now.Add(-time.Hour), "cancelled")

	// brand2 到点 scheduled（跨品牌也命中 —— 系统全局扫描）。直接建 brand2（不复用 bookingSetup
	// 以避开其硬编码 learner openid 唯一冲突）。
	brand2, _ := seedBrandWithSystemRoles(t, db)
	loc2 := seedLocation(t, db, brand2, "讯美广场B")
	instr2 := seedInstructor(t, db, brand2)
	course2 := seedPublishedCourseAt(t, db, brand2, loc2, "晚间普拉提")
	due2 := seedScheduledSession(t, db, brand2, loc2, course2, instr2, now.Add(-2*time.Hour), 8)

	ids, err := repo.ListDueSessionIDs(context.Background(), now)
	if err != nil {
		t.Fatalf("ListDueSessionIDs: %v", err)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, want := range []int64{due1, dueIP, due2} {
		if !got[want] {
			t.Errorf("due session %d missing from result", want)
		}
	}
	for _, no := range []int64{future, done, cancelled} {
		if got[no] {
			t.Errorf("non-due session %d wrongly returned", no)
		}
	}
}
