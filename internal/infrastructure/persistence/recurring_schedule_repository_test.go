package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/coursetemplate"
	domainrec "github.com/zkw/mini-schedule/backend/internal/domain/recurringschedule"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// occ 构造一节待生成 occurrence（UTC）。
func occ(start time.Time, durationMin int) domainrec.Occurrence {
	return domainrec.Occurrence{
		StartsAt:  start,
		EndsAt:    start.Add(time.Duration(durationMin) * time.Minute),
		DateLabel: start.Format("2006-01-02"),
		TimeLabel: start.Format("15:04"),
	}
}

func baseGenInput(brandID, loc, course, instr int64, occs []domainrec.Occurrence) domainrec.GenerateInput {
	rw := 2
	return domainrec.GenerateInput{
		BrandID: brandID, ActorID: 1, CourseID: course, LocationID: loc, InstructorProfileID: instr,
		Weekdays: []int{1, 3}, StartDate: "2099-01-05", RepeatWeeks: &rw,
		StartTime: "09:00", DurationMin: 60, Occurrences: occs,
	}
}

func TestRecurring_GenerateHappy(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽") // default_capacity 8

	t0 := tomorrow(9)
	occs := []domainrec.Occurrence{occ(t0, 60), occ(t0.Add(7 * 24 * time.Hour), 60), occ(t0.Add(14 * 24 * time.Hour), 60)}
	res, err := repo.Generate(context.Background(), baseGenInput(brandID, loc, course, instr, occs))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.Created) != 3 || len(res.Skipped) != 0 {
		t.Fatalf("expected 3 created / 0 skipped, got %d/%d", len(res.Created), len(res.Skipped))
	}
	if res.Schedule.SessionCount != 3 || res.Schedule.Capacity != 8 {
		t.Fatalf("schedule session_count=%d capacity=%d", res.Schedule.SessionCount, res.Schedule.Capacity)
	}
	if len(res.Schedule.Weekdays) != 2 {
		t.Fatalf("weekdays not persisted: %v", res.Schedule.Weekdays)
	}
	// 生成的场次都回填 recurring_schedule_id。
	for _, s := range res.Created {
		if s.Status != "scheduled" {
			t.Fatalf("session status = %s", s.Status)
		}
	}
}

func TestRecurring_PartialConflictSkips(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	t0 := tomorrow(9)
	// 预置一节占用 t0 的 scheduled 场次（同教练）。
	if err := db.Exec(
		`INSERT INTO class_sessions (brand_id, location_id, course_id, instructor_profile_id, starts_at, ends_at, capacity, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'scheduled')`,
		brandID, loc, course, instr, t0, t0.Add(time.Hour), 8,
	).Error; err != nil {
		t.Fatalf("seed conflict session: %v", err)
	}

	occs := []domainrec.Occurrence{occ(t0, 60), occ(t0.Add(7 * 24 * time.Hour), 60)}
	res, err := repo.Generate(context.Background(), baseGenInput(brandID, loc, course, instr, occs))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.Created) != 1 || len(res.Skipped) != 1 {
		t.Fatalf("expected 1 created / 1 skipped, got %d/%d", len(res.Created), len(res.Skipped))
	}
	if res.Skipped[0].Reason != domainrec.SkipInstructorConflict {
		t.Fatalf("skip reason = %s, want instructor_conflict", res.Skipped[0].Reason)
	}
}

func TestRecurring_AllConflictAbortsNoRow(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	t0 := tomorrow(9)
	if err := db.Exec(
		`INSERT INTO class_sessions (brand_id, location_id, course_id, instructor_profile_id, starts_at, ends_at, capacity, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'scheduled')`,
		brandID, loc, course, instr, t0, t0.Add(time.Hour), 8,
	).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	occs := []domainrec.Occurrence{occ(t0, 60)} // 唯一 occurrence 撞冲突
	_, err := repo.Generate(context.Background(), baseGenInput(brandID, loc, course, instr, occs))
	assertAppCode(t, err, apperr.ErrRecurringAllConflict)

	// 不留空壳 recurring 行。
	var n int64
	db.Table("recurring_schedules").Where("brand_id = ?", brandID).Count(&n)
	if n != 0 {
		t.Fatalf("all-conflict should not persist recurring row, found %d", n)
	}
}

func TestRecurring_ResourceBindingCapacityDefault(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")
	resID := seedResource(t, db, brandID, loc, "教室", 20)

	t0 := tomorrow(9)
	in := baseGenInput(brandID, loc, course, instr, []domainrec.Occurrence{occ(t0, 60)})
	in.LocationResourceID = &resID // capacity 未给 → 默认资源容量 20
	res, err := repo.Generate(context.Background(), in)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Schedule.Capacity != 20 {
		t.Fatalf("capacity should default to resource 20, got %d", res.Schedule.Capacity)
	}
}

func TestRecurring_BatchValidationUnpublishedCourse(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	tplRepo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	c, _ := tplRepo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "草稿课", DurationMin: 60, DefaultCapacity: 8, LocationIDs: []int64{loc},
	})

	t0 := tomorrow(9)
	_, err := repo.Generate(context.Background(), baseGenInput(brandID, loc, c.ID, instr, []domainrec.Occurrence{occ(t0, 60)}))
	assertAppCode(t, err, apperr.ErrCourseNotActive)
	var n int64
	db.Table("recurring_schedules").Where("brand_id = ?", brandID).Count(&n)
	if n != 0 {
		t.Fatalf("validation failure should not persist recurring row, found %d", n)
	}
}

func TestRecurring_CancelNonCascading(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	t0 := tomorrow(9)
	res, _ := repo.Generate(context.Background(), baseGenInput(brandID, loc, course, instr, []domainrec.Occurrence{occ(t0, 60)}))

	cancelled, err := repo.Cancel(context.Background(), brandID, 1, res.Schedule.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != domainrec.StatusCancelled {
		t.Fatalf("status = %s", cancelled.Status)
	}
	// 非级联：已生成场次仍 scheduled。
	var scheduledCount int64
	db.Model(&ClassSessionModel{}).Where("recurring_schedule_id = ? AND status = 'scheduled'", res.Schedule.ID).Count(&scheduledCount)
	if scheduledCount != 1 {
		t.Fatalf("cancel should NOT cascade; expected 1 scheduled session, got %d", scheduledCount)
	}
	// 再取消 → 不允许。
	if _, err := repo.Cancel(context.Background(), brandID, 1, res.Schedule.ID); codeOf(err) != apperr.ErrRecurringCancelNotAllowed {
		t.Fatalf("re-cancel should be RECURRING_CANCEL_NOT_ALLOWED, got %v", err)
	}
}

func TestRecurring_CountActiveReferencesIncludesRecurring(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRecurringScheduleRepository(db)
	locRepo := NewLocationRepository(db, nil)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	t0 := tomorrow(9)
	res, _ := repo.Generate(context.Background(), baseGenInput(brandID, loc, course, instr, []domainrec.Occurrence{occ(t0, 60)}))

	n, _ := locRepo.CountActiveReferences(context.Background(), brandID, loc)
	if n < 1 {
		t.Fatalf("active recurring should be counted, got %d", n)
	}
	// 取消后 recurring 不再计入（但生成的场次仍计入，故仍 >=1）。
	_, _ = repo.Cancel(context.Background(), brandID, 1, res.Schedule.ID)
	// 取消生成的那节场次后，引用应归零。
	db.Model(&ClassSessionModel{}).Where("recurring_schedule_id = ?", res.Schedule.ID).Update("status", "cancelled")
	n2, _ := locRepo.CountActiveReferences(context.Background(), brandID, loc)
	if n2 != 0 {
		t.Fatalf("after cancel recurring + sessions, refs should be 0, got %d", n2)
	}
}
