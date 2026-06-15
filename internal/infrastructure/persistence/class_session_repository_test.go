package persistence

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	"github.com/zkw/mini-schedule/backend/internal/domain/coursetemplate"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// seedInstructor inserts an active, schedulable instructor profile and returns its id.
func seedInstructor(t *testing.T, db *gorm.DB, brandID int64) int64 {
	t.Helper()
	buID := seedBrandUser(t, db, brandID)
	if err := db.Exec(
		`INSERT INTO instructor_profiles (brand_id, brand_user_id, display_name, is_schedulable, status)
		 VALUES (?, ?, '教练A', TRUE, 'active')`,
		brandID, buID,
	).Error; err != nil {
		t.Fatalf("seed instructor: %v", err)
	}
	var id int64
	if err := db.Raw(`SELECT id FROM instructor_profiles WHERE brand_user_id = ?`, buID).Scan(&id).Error; err != nil {
		t.Fatalf("read instructor id: %v", err)
	}
	return id
}

// seedPublishedCourseAt creates a course template available at locationID and publishes it.
func seedPublishedCourseAt(t *testing.T, db *gorm.DB, brandID, locationID int64, title string) int64 {
	t.Helper()
	repo := NewCourseTemplateRepository(db)
	c, err := repo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: title, DurationMin: 60, DefaultCapacity: 8,
		LocationIDs: []int64{locationID},
	})
	if err != nil {
		t.Fatalf("seed course: %v", err)
	}
	if _, err := repo.UpdateStatus(context.Background(), brandID, 1, c.ID, coursetemplate.StatusPublished); err != nil {
		t.Fatalf("publish course: %v", err)
	}
	return c.ID
}

func tomorrow(hour int) time.Time {
	return time.Now().UTC().Add(24 * time.Hour).Truncate(time.Hour).Add(time.Duration(hour) * time.Hour)
}

func TestClassSession_CreateHappy(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewClassSessionRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "晨间瑜伽")

	start := tomorrow(9)
	got, err := repo.Create(context.Background(), classsession.CreateInput{
		BrandID: brandID, ActorID: 1, CourseID: course, LocationID: loc, InstructorProfileID: instr,
		StartsAt: start, EndsAt: start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Status != classsession.StatusScheduled {
		t.Errorf("status = %s, want scheduled", got.Status)
	}
	if got.Capacity != 8 {
		t.Errorf("capacity = %d, want default 8", got.Capacity)
	}
	if got.CourseTitle != "晨间瑜伽" || got.LocationName != "门店1" || got.InstructorName != "教练A" {
		t.Errorf("denormalized names wrong: %q / %q / %q", got.CourseTitle, got.LocationName, got.InstructorName)
	}
}

func TestClassSession_CourseNotPublished(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewClassSessionRepository(db)
	tplRepo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	// draft course (not published)
	c, _ := tplRepo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "草稿课", DurationMin: 60, DefaultCapacity: 8, LocationIDs: []int64{loc},
	})

	start := tomorrow(9)
	_, err := repo.Create(context.Background(), classsession.CreateInput{
		BrandID: brandID, ActorID: 1, CourseID: c.ID, LocationID: loc, InstructorProfileID: instr,
		StartsAt: start, EndsAt: start.Add(time.Hour),
	})
	if codeOf(err) != apperr.ErrCourseNotActive {
		t.Fatalf("want COURSE_NOT_ACTIVE, got %v", err)
	}
}

func TestClassSession_LocationUnavailable(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewClassSessionRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc1 := seedLocation(t, db, brandID, "门店1")
	loc2 := seedLocation(t, db, brandID, "门店2")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc1, "仅门店1") // available only at loc1

	start := tomorrow(9)
	_, err := repo.Create(context.Background(), classsession.CreateInput{
		BrandID: brandID, ActorID: 1, CourseID: course, LocationID: loc2, InstructorProfileID: instr,
		StartsAt: start, EndsAt: start.Add(time.Hour),
	})
	if codeOf(err) != apperr.ErrCourseLocationUnavailable {
		t.Fatalf("want COURSE_LOCATION_UNAVAILABLE, got %v", err)
	}
}

func TestClassSession_InstructorConflict(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewClassSessionRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	start := tomorrow(9)
	in := classsession.CreateInput{
		BrandID: brandID, ActorID: 1, CourseID: course, LocationID: loc, InstructorProfileID: instr,
		StartsAt: start, EndsAt: start.Add(time.Hour),
	}
	if _, err := repo.Create(context.Background(), in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// overlapping session for same instructor → EXCLUDE 23P01
	in2 := in
	in2.StartsAt = start.Add(30 * time.Minute)
	in2.EndsAt = start.Add(90 * time.Minute)
	_, err := repo.Create(context.Background(), in2)
	if codeOf(err) != apperr.ErrSessionInstructorConflict {
		t.Fatalf("want SESSION_INSTRUCTOR_CONFLICT, got %v", err)
	}
}

func TestClassSession_CancelAndNotAllowed(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewClassSessionRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	start := tomorrow(9)
	c, _ := repo.Create(context.Background(), classsession.CreateInput{
		BrandID: brandID, ActorID: 1, CourseID: course, LocationID: loc, InstructorProfileID: instr,
		StartsAt: start, EndsAt: start.Add(time.Hour),
	})
	got, err := repo.Cancel(context.Background(), brandID, 1, c.ID, "教练请假")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != classsession.StatusCancelled {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	// second cancel rejected
	if _, err := repo.Cancel(context.Background(), brandID, 1, c.ID, "再取消"); codeOf(err) != apperr.ErrSessionCancelNotAllowed {
		t.Fatalf("want SESSION_CANCEL_NOT_ALLOWED, got %v", err)
	}
}

func TestCountActiveReferences_IncludesScheduledSession(t *testing.T) {
	db := newMigratedTestDB(t)
	locRepo := NewLocationRepository(db, nil)
	sessRepo := NewClassSessionRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	start := tomorrow(9)
	if _, err := sessRepo.Create(context.Background(), classsession.CreateInput{
		BrandID: brandID, ActorID: 1, CourseID: course, LocationID: loc, InstructorProfileID: instr,
		StartsAt: start, EndsAt: start.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	n, err := locRepo.CountActiveReferences(context.Background(), brandID, loc)
	if err != nil {
		t.Fatalf("count refs: %v", err)
	}
	if n < 1 {
		t.Errorf("CountActiveReferences should include scheduled session, got %d", n)
	}
}
