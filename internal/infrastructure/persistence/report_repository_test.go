package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	domainreport "github.com/zkw/mini-schedule/backend/internal/domain/report"
)

// rptSeq 单调计数，保证 seedLearnerProfile 的 openID 在测试内唯一（避免 timestamp 碰撞）。
var rptSeq int

// ---- raw 造数据 helper（精确控制 status/时间戳，绕过业务流）----

func rptBooking(t *testing.T, db *gorm.DB, brandID, sessionID, learnerID int64, status string, bookedAt time.Time) int64 {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO bookings (brand_id, class_session_id, brand_learner_profile_id, source, status, booked_at)
		 VALUES (?, ?, ?, 'staff_assisted', ?, ?)`,
		brandID, sessionID, learnerID, status, bookedAt,
	).Error; err != nil {
		t.Fatalf("seed booking(%s): %v", status, err)
	}
	var id int64
	db.Raw(`SELECT id FROM bookings WHERE brand_id = ? ORDER BY id DESC LIMIT 1`, brandID).Scan(&id)
	return id
}

func rptHold(t *testing.T, db *gorm.DB, brandID, bookingID, entID, learnerID int64, heldAt time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO entitlement_holds (brand_id, booking_id, learner_entitlement_id, brand_learner_profile_id, credits, status, held_at)
		 VALUES (?, ?, ?, ?, 1, 'held', ?)`,
		brandID, bookingID, entID, learnerID, heldAt,
	).Error; err != nil {
		t.Fatalf("seed hold: %v", err)
	}
}

func rptConsumption(t *testing.T, db *gorm.DB, brandID, bookingID, entID, learnerID int64, consumedAt time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO entitlement_consumptions (brand_id, learner_entitlement_id, booking_id, brand_learner_profile_id, credits, consumption_type, consumed_at)
		 VALUES (?, ?, ?, ?, 1, 'attendance', ?)`,
		brandID, entID, bookingID, learnerID, consumedAt,
	).Error; err != nil {
		t.Fatalf("seed consumption: %v", err)
	}
}

func rptWaitlist(t *testing.T, db *gorm.DB, brandID, sessionID, learnerID int64, status string, position int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO waitlist_entries (brand_id, class_session_id, brand_learner_profile_id, position, status)
		 VALUES (?, ?, ?, ?, ?)`,
		brandID, sessionID, learnerID, position, status,
	).Error; err != nil {
		t.Fatalf("seed waitlist(%s): %v", status, err)
	}
}

func rptLearner(t *testing.T, db *gorm.DB, brandID int64, n int) int64 {
	t.Helper()
	rptSeq++
	return seedLearnerProfile(t, db, brandID, fmt.Sprintf("rpt:%d:%d", rptSeq, n))
}

// TestBrandOverview_CoreCounts 全口径核心断言：A 组锚定场次 starts_at、B 组锚定事件时间戳、C 组 live。
func TestBrandOverview_CoreCounts(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewReportRepository(db)
	now := time.Now().UTC()
	from, to := now.Add(-7*24*time.Hour), now.Add(time.Hour)

	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc1 := seedLocation(t, db, brandID, "讯美广场")
	loc2 := seedLocation(t, db, brandID, "科技园")
	instr1 := seedInstructor(t, db, brandID)
	instr2 := seedInstructor(t, db, brandID)
	courseA := seedPublishedCourseAt(t, db, brandID, loc1, "晨间瑜伽")
	courseB := seedPublishedCourseAt(t, db, brandID, loc2, "晚间普拉提")

	// 场次：sIn1/sIn2 完成（占座分母）、sIn3 scheduled（不入占座分母）、sOut 窗外。
	sIn1 := seedScheduledSession(t, db, brandID, loc1, courseA, instr1, now.Add(-2*24*time.Hour), 10)
	setSessionTimesStatus(t, db, sIn1, now.Add(-2*24*time.Hour), now.Add(-2*24*time.Hour+time.Hour), "completed")
	sIn2 := seedScheduledSession(t, db, brandID, loc1, courseA, instr1, now.Add(-24*time.Hour), 10)
	setSessionTimesStatus(t, db, sIn2, now.Add(-24*time.Hour), now.Add(-24*time.Hour+time.Hour), "completed")
	sIn3 := seedScheduledSession(t, db, brandID, loc2, courseB, instr2, now.Add(-24*time.Hour), 5) // scheduled
	sOut := seedScheduledSession(t, db, brandID, loc1, courseA, instr1, now.Add(-40*24*time.Hour), 10)
	setSessionTimesStatus(t, db, sOut, now.Add(-40*24*time.Hour), now.Add(-40*24*time.Hour+time.Hour), "completed")

	l := func(n int) int64 { return rptLearner(t, db, brandID, n) }
	// sIn1: 1 attended / 1 cancelled / 1 no_show / 1 pending / 1 booked
	bkAtt := rptBooking(t, db, brandID, sIn1, l(1), "attended", now.Add(-2*24*time.Hour))
	rptBooking(t, db, brandID, sIn1, l(2), "cancelled", now.Add(-2*24*time.Hour))
	rptBooking(t, db, brandID, sIn1, l(3), "no_show", now.Add(-2*24*time.Hour))
	rptBooking(t, db, brandID, sIn1, l(4), "pending_no_show", now.Add(-2*24*time.Hour))
	rptBooking(t, db, brandID, sIn1, l(5), "booked", now.Add(-2*24*time.Hour))
	// sIn2: 2 attended
	bkC := rptBooking(t, db, brandID, sIn2, l(6), "attended", now.Add(-24*time.Hour))
	bkD := rptBooking(t, db, brandID, sIn2, l(7), "attended", now.Add(-24*time.Hour))
	// sIn3: 1 booked
	rptBooking(t, db, brandID, sIn3, l(8), "booked", now.Add(-24*time.Hour))
	// sOut: 1 attended（窗外，A 组不计）+ 1 pending（C 组 live 计）
	rptBooking(t, db, brandID, sOut, l(9), "attended", now.Add(-40*24*time.Hour))
	rptBooking(t, db, brandID, sOut, l(10), "pending_no_show", now.Add(-40*24*time.Hour))

	// B 组：权益 hold/consumption，窗内 1 + 窗外 1。
	ent := grantEnt(t, db, brandID, l(1), "次卡", "class_pack", intp(10), "all", "all", nil, nil)
	rptHold(t, db, brandID, bkAtt, ent, l(1), now.Add(-2*24*time.Hour))       // 窗内（holds 唯一 booking）
	rptHold(t, db, brandID, bkD, ent, l(1), now.Add(-40*24*time.Hour))        // 窗外
	rptConsumption(t, db, brandID, bkC, ent, l(1), now.Add(-24*time.Hour))    // 窗内
	rptConsumption(t, db, brandID, bkD, ent, l(1), now.Add(-40*24*time.Hour)) // 窗外

	// C 组 waitlist：waiting + eligible 计；cancelled 不计；窗外也计（live）。
	rptWaitlist(t, db, brandID, sIn3, l(11), "waiting", 1)
	rptWaitlist(t, db, brandID, sIn3, l(12), "eligible_to_promote", 2)
	rptWaitlist(t, db, brandID, sIn3, l(13), "cancelled", 3)
	rptWaitlist(t, db, brandID, sOut, l(14), "waiting", 1)

	o, err := repo.BrandOverviewCounts(context.Background(), domainreport.ReportQuery{BrandID: brandID, From: from, To: to})
	if err != nil {
		t.Fatalf("BrandOverviewCounts: %v", err)
	}

	eq := func(name string, got, want int64) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	eq("bookings_total", o.BookingsTotal, 7)
	eq("attended_total", o.AttendedTotal, 3)
	eq("cancelled_total", o.CancelledTotal, 1)
	eq("no_show_total", o.NoShowTotal, 1)
	eq("total_capacity", o.TotalCapacity, 20)
	eq("attended_in_completed", o.AttendedInCompleted, 3)
	if o.OccupancyRate < 0.149 || o.OccupancyRate > 0.151 {
		t.Errorf("occupancy_rate = %v, want ~0.15", o.OccupancyRate)
	}
	eq("entitlement_locked_total", o.EntitlementLockedTotal, 1)
	eq("entitlement_consumed_total", o.EntitlementConsumedTotal, 1)
	eq("pending_no_show_total", o.PendingNoShowTotal, 2) // live：sIn1 + sOut
	eq("waitlist_total", o.WaitlistTotal, 3)             // live：sIn3 两个 + sOut 一个，cancelled 不计

	// 热门课程：courseA=6（sIn1 非取消4 + sIn2 2）、courseB=1。
	if len(o.PopularCourses) != 2 || o.PopularCourses[0].CourseID != courseA || o.PopularCourses[0].BookingCount != 6 {
		t.Errorf("popular_courses = %+v, want A=6 first", o.PopularCourses)
	}
	if o.PopularCourses[1].CourseID != courseB || o.PopularCourses[1].BookingCount != 1 {
		t.Errorf("popular_courses[1] = %+v, want B=1", o.PopularCourses[1])
	}
	// Location 分布：loc1 场次2/预约6、loc2 场次1/预约1。
	dist := map[int64]domainreport.LocationDistribution{}
	for _, d := range o.LocationDistribution {
		dist[d.LocationID] = d
	}
	if d := dist[loc1]; d.SessionCount != 2 || d.BookingCount != 6 {
		t.Errorf("loc1 dist = %+v, want sess2/bk6", d)
	}
	if d := dist[loc2]; d.SessionCount != 1 || d.BookingCount != 1 {
		t.Errorf("loc2 dist = %+v, want sess1/bk1", d)
	}
	// Instructor 场次：instr1=2、instr2=1。
	isess := map[int64]int64{}
	for _, s := range o.InstructorSessions {
		isess[s.InstructorProfileID] = s.SessionCount
	}
	eq("instr1 sessions", isess[instr1], 2)
	eq("instr2 sessions", isess[instr2], 1)
}

// TestBrandOverview_DataScope 店长本店收紧（含 location-less waitlist 经 join 收紧）。
func TestBrandOverview_DataScope(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewReportRepository(db)
	now := time.Now().UTC()
	from, to := now.Add(-7*24*time.Hour), now.Add(time.Hour)

	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc1 := seedLocation(t, db, brandID, "本店")
	loc2 := seedLocation(t, db, brandID, "他店")
	instr1 := seedInstructor(t, db, brandID)
	instr2 := seedInstructor(t, db, brandID)
	courseA := seedPublishedCourseAt(t, db, brandID, loc1, "课A")
	courseB := seedPublishedCourseAt(t, db, brandID, loc2, "课B")

	sL1 := seedScheduledSession(t, db, brandID, loc1, courseA, instr1, now.Add(-24*time.Hour), 10)
	setSessionTimesStatus(t, db, sL1, now.Add(-24*time.Hour), now.Add(-23*time.Hour), "completed")
	sL2 := seedScheduledSession(t, db, brandID, loc2, courseB, instr2, now.Add(-24*time.Hour), 10)
	setSessionTimesStatus(t, db, sL2, now.Add(-24*time.Hour), now.Add(-23*time.Hour), "completed")

	l := func(n int) int64 { return rptLearner(t, db, brandID, n) }
	rptBooking(t, db, brandID, sL1, l(1), "attended", now.Add(-24*time.Hour))
	rptBooking(t, db, brandID, sL1, l(2), "booked", now.Add(-24*time.Hour))
	rptBooking(t, db, brandID, sL2, l(3), "attended", now.Add(-24*time.Hour))
	rptWaitlist(t, db, brandID, sL1, l(4), "waiting", 1) // loc1
	rptWaitlist(t, db, brandID, sL2, l(5), "waiting", 1) // loc2

	// nil scope = 全品牌
	all, err := repo.BrandOverviewCounts(context.Background(), domainreport.ReportQuery{BrandID: brandID, From: from, To: to})
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if all.BookingsTotal != 3 || all.WaitlistTotal != 2 {
		t.Errorf("nil-scope = bk%d wl%d, want bk3 wl2", all.BookingsTotal, all.WaitlistTotal)
	}

	// scope=[loc1] 店长本店
	scoped, err := repo.BrandOverviewCounts(context.Background(), domainreport.ReportQuery{BrandID: brandID, ScopeLocationIDs: []int64{loc1}, From: from, To: to})
	if err != nil {
		t.Fatalf("scoped: %v", err)
	}
	if scoped.BookingsTotal != 2 {
		t.Errorf("scoped bookings = %d, want 2 (仅 loc1)", scoped.BookingsTotal)
	}
	if scoped.WaitlistTotal != 1 {
		t.Errorf("scoped waitlist = %d, want 1 (location-less 经 join 收紧)", scoped.WaitlistTotal)
	}

	// 空 scope = 拒绝所有 → 全 0
	empty, err := repo.BrandOverviewCounts(context.Background(), domainreport.ReportQuery{BrandID: brandID, ScopeLocationIDs: []int64{}, From: from, To: to})
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if empty.BookingsTotal != 0 || empty.WaitlistTotal != 0 {
		t.Errorf("empty-scope = bk%d wl%d, want all 0", empty.BookingsTotal, empty.WaitlistTotal)
	}
}

// TestBrandOverview_OccupancyDivZero 窗口无 completed 场次 → 上座率 0，不除零。
func TestBrandOverview_OccupancyDivZero(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewReportRepository(db)
	now := time.Now().UTC()
	from, to := now.Add(-7*24*time.Hour), now.Add(2*time.Hour)

	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "课")
	// 仅 scheduled 场次（无 completed）。
	s := seedScheduledSession(t, db, brandID, loc, course, instr, now.Add(-time.Hour), 10)
	rptBooking(t, db, brandID, s, rptLearner(t, db, brandID, 1), "booked", now.Add(-time.Hour))

	o, err := repo.BrandOverviewCounts(context.Background(), domainreport.ReportQuery{BrandID: brandID, From: from, To: to})
	if err != nil {
		t.Fatalf("BrandOverviewCounts: %v", err)
	}
	if o.TotalCapacity != 0 || o.OccupancyRate != 0 {
		t.Errorf("occupancy divzero: capacity=%d rate=%v, want 0/0", o.TotalCapacity, o.OccupancyRate)
	}
	if o.BookingsTotal != 1 {
		t.Errorf("bookings_total = %d, want 1", o.BookingsTotal)
	}
}
