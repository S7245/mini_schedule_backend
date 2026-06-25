package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
)

// TestBooking_ListFilterStatuses 多状态 IN 过滤（Batch 14b 上课记录）：Statuses=[attended,no_show] 仅返回终态；
// 单 Status 仍生效（brand 侧回归）。
func TestBooking_ListFilterStatuses(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewBookingRepository(db)
	f := bookingSetup(t, db)

	mkSession := func() int64 {
		return seedScheduledSession(t, db, f.brandID, f.loc, f.course, seedInstructor(t, db, f.brandID), time.Now().UTC().Add(48*time.Hour), 8)
	}
	insBk := func(sess int64, status string) {
		if err := db.Exec(
			`INSERT INTO bookings (brand_id, class_session_id, brand_learner_profile_id, source, status, booked_at)
			 VALUES (?, ?, ?, 'staff_assisted', ?, NOW())`,
			f.brandID, sess, f.learner, status,
		).Error; err != nil {
			t.Fatalf("ins booking %s: %v", status, err)
		}
	}
	// 各 booking 用独立 session（避开 bookings partial unique(session,learner) WHERE status<>'cancelled'）。
	insBk(mkSession(), "booked")
	insBk(mkSession(), "cancelled")
	insBk(mkSession(), "attended")
	insBk(mkSession(), "no_show")

	// 多状态：attended + no_show。
	items, total, err := repo.List(context.Background(), booking.ListFilter{
		BrandID: f.brandID, BrandLearnerProfileID: f.learner,
		Statuses: []string{"attended", "no_show"},
	}, 0, 50)
	if err != nil {
		t.Fatalf("list multi: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("multi-status total=%d (items %d), want 2", total, len(items))
	}
	for _, b := range items {
		if b.Status != booking.StatusAttended && b.Status != booking.StatusNoShow {
			t.Errorf("got non-terminal status %s", b.Status)
		}
	}

	// 单 Status 仍生效（回归）。
	one, t1, err := repo.List(context.Background(), booking.ListFilter{
		BrandID: f.brandID, BrandLearnerProfileID: f.learner, Status: "booked",
	}, 0, 50)
	if err != nil {
		t.Fatalf("list single: %v", err)
	}
	if t1 != 1 || len(one) != 1 || one[0].Status != booking.StatusBooked {
		t.Fatalf("single status total=%d, want 1 booked", t1)
	}
}
