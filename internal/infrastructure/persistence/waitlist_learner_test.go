package persistence

import (
	"context"
	"testing"

	"github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
)

// TestLearnerWaitlist_Join 学员自助加入候补（Batch 14b）：operated_by NULL + audit actor=learner + position；
// staff 代加入（SelfService=false）operated_by=actor 回归。
func TestLearnerWaitlist_Join(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, _ := fullSession(t, db, f, instr2) // fullSession 用 f.learner 占满，故自助用另一学员
	self := seedLearnerProfile(t, db, f.brandID, "wl:self")

	// 学员自助 join：operated_by NULL + audit learner。
	e, err := repo.Join(context.Background(), waitlist.JoinInput{
		BrandID: f.brandID, ClassSessionID: sess, BrandLearnerProfileID: self, SelfService: true,
	})
	if err != nil {
		t.Fatalf("self join: %v", err)
	}
	if e.Position != 1 || e.Status != waitlist.StatusWaiting {
		t.Errorf("shape position=%d status=%s, want 1/waiting", e.Position, e.Status)
	}
	var opBy *int64
	db.Raw(`SELECT operated_by FROM waitlist_entries WHERE id = ?`, e.ID).Row().Scan(&opBy)
	if opBy != nil {
		t.Errorf("self-service operated_by should be NULL, got %v", *opBy)
	}
	var actorType string
	var actorID int64
	db.Raw(`SELECT actor_type, actor_id FROM operation_logs WHERE action = 'waitlist_joined' ORDER BY id DESC LIMIT 1`).Row().Scan(&actorType, &actorID)
	if actorType != "learner" || actorID != self {
		t.Errorf("audit actor=%s/%d, want learner/%d", actorType, actorID, self)
	}

	// staff 代加入（回归）：operated_by=actor。
	l2 := seedLearnerProfile(t, db, f.brandID, "wl:staff")
	e2, err := repo.Join(context.Background(), waitlist.JoinInput{
		BrandID: f.brandID, ActorID: f.actor, ClassSessionID: sess, BrandLearnerProfileID: l2,
	})
	if err != nil {
		t.Fatalf("staff join: %v", err)
	}
	var opBy2 *int64
	db.Raw(`SELECT operated_by FROM waitlist_entries WHERE id = ?`, e2.ID).Row().Scan(&opBy2)
	if opBy2 == nil || *opBy2 != f.actor {
		t.Errorf("staff operated_by=%v, want %d", opBy2, f.actor)
	}
}
