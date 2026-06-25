package persistence

import (
	"context"
	"testing"

	"github.com/zkw/mini-schedule/backend/internal/domain/learner"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// TestBridge_FindOrCreateProfileByOpenID 桥接（Batch 14a）：C 端微信登录 by-openid find-or-create
// identity(phone NULL) + profile(by brand+identity)，幂等。
func TestBridge_FindOrCreateProfileByOpenID(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)
	ctx := context.Background()

	// 1) 新建：identity(by openid, phone NULL) + profile。
	p1, err := repo.FindOrCreateProfileByOpenID(ctx, brandID, "dev_alice", "Alice")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p1.ID == 0 || p1.Status != learner.StatusActive {
		t.Fatalf("bad profile: %+v", p1)
	}
	var openid string
	var phone *string
	if err := db.Raw(
		`SELECT li.wechat_open_id, li.phone FROM learner_identities li
		 JOIN brand_learner_profiles p ON p.learner_identity_id = li.id WHERE p.id = ?`, p1.ID,
	).Row().Scan(&openid, &phone); err != nil {
		t.Fatalf("scan identity: %v", err)
	}
	if openid != "dev_alice" {
		t.Errorf("openid = %q, want dev_alice", openid)
	}
	if phone != nil {
		t.Errorf("phone should be NULL, got %q", *phone)
	}

	// 2) 幂等：复登同 openid → 同 profile，无重复行。
	p2, err := repo.FindOrCreateProfileByOpenID(ctx, brandID, "dev_alice", "Alice-renamed")
	if err != nil {
		t.Fatalf("idempotent: %v", err)
	}
	if p2.ID != p1.ID {
		t.Errorf("idempotent profile id %d != %d", p2.ID, p1.ID)
	}
	var idCount, profCount int64
	db.Raw(`SELECT count(*) FROM learner_identities WHERE wechat_open_id = 'dev_alice'`).Scan(&idCount)
	db.Raw(`SELECT count(*) FROM brand_learner_profiles WHERE brand_id = ?`, brandID).Scan(&profCount)
	if idCount != 1 || profCount != 1 {
		t.Errorf("duplicate rows: identities=%d profiles=%d, want 1/1", idCount, profCount)
	}

	// 3) 不同 openid → 不同 profile（同 brand）。
	pb, err := repo.FindOrCreateProfileByOpenID(ctx, brandID, "dev_bob", "Bob")
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	if pb.ID == p1.ID {
		t.Error("bob should be a distinct profile")
	}
}

// TestBridge_FindOrCreateProfileByOpenID_Quota 首登建 profile honor max_learners 硬限；复登幂等不消耗配额。
func TestBridge_FindOrCreateProfileByOpenID_Quota(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 1) // max_learners = 1
	ctx := context.Background()

	if _, err := repo.FindOrCreateProfileByOpenID(ctx, brandID, "dev_a", "A"); err != nil {
		t.Fatalf("first within quota: %v", err)
	}
	// 复登 a 幂等，不消耗配额。
	if _, err := repo.FindOrCreateProfileByOpenID(ctx, brandID, "dev_a", "A"); err != nil {
		t.Fatalf("re-login a (idempotent): %v", err)
	}
	// 第二个学员超额。
	_, err := repo.FindOrCreateProfileByOpenID(ctx, brandID, "dev_b", "B")
	assertAppCode(t, err, apperr.ErrQuotaExceeded)
}
