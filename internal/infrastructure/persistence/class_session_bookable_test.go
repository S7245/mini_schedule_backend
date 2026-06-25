package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/classsession"
)

// TestClassSession_ListBookableFilter 锁定 C 端课程表读语义（Batch 14a）：
// learnerbooking.Service 复用 repo.List(Status="scheduled", From=now) —— 仅返回 brand+scheduled+未来场次，
// 排除 draft / completed / cancelled / 已过去的场次。
func TestClassSession_ListBookableFilter(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewClassSessionRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")

	now := time.Now().UTC()
	seed := func(startsAt time.Time, status string) {
		if err := db.Exec(
			`INSERT INTO class_sessions (brand_id, location_id, course_id, instructor_profile_id, starts_at, ends_at, capacity, booked_count, status)
			 VALUES (?, ?, ?, ?, ?, ?, 10, 0, ?)`,
			brandID, loc, course, instr, startsAt, startsAt.Add(time.Hour), status,
		).Error; err != nil {
			t.Fatalf("seed session(%s): %v", status, err)
		}
	}
	seed(now.Add(24*time.Hour), "scheduled")  // ✅ 应返回
	seed(now.Add(48*time.Hour), "scheduled")  // ✅ 应返回
	seed(now.Add(-24*time.Hour), "scheduled") // ✗ 已过去
	seed(now.Add(24*time.Hour), "draft")      // ✗ 未发布
	seed(now.Add(24*time.Hour), "completed")  // ✗ 已完成
	seed(now.Add(24*time.Hour), "cancelled")  // ✗ 已取消

	from := now
	items, total, err := repo.List(context.Background(), classsession.ListFilter{
		BrandID: brandID,
		Status:  string(classsession.StatusScheduled),
		From:    &from,
	}, 0, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("bookable total = %d (items %d), want 2", total, len(items))
	}
	for _, s := range items {
		if s.Status != classsession.StatusScheduled {
			t.Errorf("got status %s, want scheduled", s.Status)
		}
		if !s.StartsAt.After(now) {
			t.Errorf("got past session %v", s.StartsAt)
		}
	}
	// 排序：soonest first。
	if items[0].StartsAt.After(items[1].StartsAt) {
		t.Errorf("not ordered starts_at ASC: %v then %v", items[0].StartsAt, items[1].StartsAt)
	}
}
