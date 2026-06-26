package persistence

import (
	"context"
	"testing"

	"github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
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

// TestLearnerWaitlist_ListAndCancel 我的候补（仅本 profile + 反范式）+ 自助取消（所有权 + operated_by NULL + audit learner）。
func TestLearnerWaitlist_ListAndCancel(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewWaitlistRepository(db)
	f := bookingSetup(t, db)
	instr2 := seedInstructor(t, db, f.brandID)
	sess, _ := fullSession(t, db, f, instr2)
	self := seedLearnerProfile(t, db, f.brandID, "wl:self")
	bob := seedLearnerProfile(t, db, f.brandID, "wl:bob")
	ctx := context.Background()

	selfEntry, err := repo.Join(ctx, waitlist.JoinInput{BrandID: f.brandID, ClassSessionID: sess, BrandLearnerProfileID: self, SelfService: true})
	if err != nil {
		t.Fatalf("self join: %v", err)
	}
	bobEntry, err := repo.Join(ctx, waitlist.JoinInput{BrandID: f.brandID, ClassSessionID: sess, BrandLearnerProfileID: bob, SelfService: true})
	if err != nil {
		t.Fatalf("bob join: %v", err)
	}

	// ListByLearner(self) → 仅本人 + 反范式齐全。
	mine, err := repo.ListByLearner(ctx, f.brandID, self)
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != selfEntry.ID {
		t.Fatalf("ListByLearner = %d entries, want 1 (self)", len(mine))
	}
	if mine[0].CourseTitle == "" || mine[0].LocationName == "" {
		t.Errorf("denormalized missing: course=%q location=%q", mine[0].CourseTitle, mine[0].LocationName)
	}

	// ownership：self 取消 bob 的候补 → NOT_FOUND（不泄漏）。
	_, err = repo.CancelByLearner(ctx, f.brandID, self, bobEntry.ID)
	assertAppCode(t, err, apperr.ErrWaitlistEntryNotFound)

	// 本人取消 → cancelled + operated_by NULL + audit learner。
	out, err := repo.CancelByLearner(ctx, f.brandID, self, selfEntry.ID)
	if err != nil {
		t.Fatalf("self cancel: %v", err)
	}
	if out.Status != waitlist.StatusCancelled {
		t.Errorf("status = %s, want cancelled", out.Status)
	}
	var opBy *int64
	db.Raw(`SELECT operated_by FROM waitlist_entries WHERE id = ?`, selfEntry.ID).Row().Scan(&opBy)
	if opBy != nil {
		t.Errorf("cancel operated_by should be NULL, got %v", *opBy)
	}
	var actorType string
	db.Raw(`SELECT actor_type FROM operation_logs WHERE action = 'waitlist_cancelled' ORDER BY id DESC LIMIT 1`).Row().Scan(&actorType)
	if actorType != "learner" {
		t.Errorf("cancel audit actor=%q, want learner", actorType)
	}
	// 取消后不在活跃我的候补。
	mine2, _ := repo.ListByLearner(ctx, f.brandID, self)
	if len(mine2) != 0 {
		t.Errorf("after cancel ListByLearner = %d, want 0", len(mine2))
	}
}
