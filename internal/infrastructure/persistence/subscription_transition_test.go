package persistence

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"
)

func subIDForBrand(t *testing.T, db *gorm.DB, brandID int64) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(`SELECT id FROM brand_subscriptions WHERE brand_id = ? ORDER BY id DESC LIMIT 1`, brandID).
		Scan(&id).Error; err != nil {
		t.Fatalf("read sub id: %v", err)
	}
	return id
}

func subStatusOf(t *testing.T, db *gorm.DB, id int64) string {
	t.Helper()
	var s string
	db.Raw(`SELECT status FROM brand_subscriptions WHERE id = ?`, id).Scan(&s)
	return s
}

func subExpiresAt(t *testing.T, db *gorm.DB, id int64) time.Time {
	t.Helper()
	var v time.Time
	db.Raw(`SELECT expires_at FROM brand_subscriptions WHERE id = ?`, id).Scan(&v)
	return v
}

func subGraceEndsAt(t *testing.T, db *gorm.DB, id int64) *time.Time {
	t.Helper()
	var v *time.Time
	db.Raw(`SELECT grace_ends_at FROM brand_subscriptions WHERE id = ?`, id).Scan(&v)
	return v
}

// countSystemSubAudit 数系统订阅转换 audit（actor_type='system' AND actor_id IS NULL）。
func countSystemSubAudit(t *testing.T, db *gorm.DB, action string, subID int64) int {
	t.Helper()
	var n int
	db.Raw(`SELECT COUNT(*) FROM operation_logs
	        WHERE action = ? AND target_type = 'brand_subscription' AND target_id = ?
	          AND actor_type = 'system' AND actor_id IS NULL`, action, subID).Scan(&n)
	return n
}

// ---- ListSubscriptionsDueForGrace ----

func TestListSubscriptionsDueForGrace(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCommercialRepository(db)
	now := time.Now().UTC()
	tp := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	bHit, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bHit, "active", now.Add(-time.Hour), nil, 5) // active+过期 → 命中
	idHit := subIDForBrand(t, db, bHit)

	bFuture, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bFuture, "active", now.Add(time.Hour), nil, 5) // active+未来
	bGrace, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bGrace, "grace_period", now.Add(-time.Hour), tp(time.Hour), 5)
	bFrozen, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bFrozen, "frozen", now.Add(-time.Hour), nil, 5)
	bCancelled, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bCancelled, "cancelled", now.Add(-time.Hour), nil, 5)

	ids, err := repo.ListSubscriptionsDueForGrace(context.Background(), now)
	if err != nil {
		t.Fatalf("ListSubscriptionsDueForGrace: %v", err)
	}
	if !int64InSlice(idHit, ids) {
		t.Errorf("active+expired sub %d missing from due-for-grace; got %v", idHit, ids)
	}
	for _, b := range []int64{bFuture, bGrace, bFrozen, bCancelled} {
		if id := subIDForBrand(t, db, b); int64InSlice(id, ids) {
			t.Errorf("sub %d (brand %d) wrongly listed for grace", id, b)
		}
	}
}

// ---- TransitionSubscriptionToGrace ----

func TestTransitionSubscriptionToGrace(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCommercialRepository(db)
	now := time.Now().UTC()

	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, brandID, "active", now.Add(-2*time.Hour), nil, 5)
	id := subIDForBrand(t, db, brandID)
	storedExpires := subExpiresAt(t, db, id)

	ok, err := repo.TransitionSubscriptionToGrace(context.Background(), id, now, 7)
	if err != nil {
		t.Fatalf("TransitionSubscriptionToGrace: %v", err)
	}
	if !ok {
		t.Fatal("expected transitioned=true")
	}
	if got := subStatusOf(t, db, id); got != "grace_period" {
		t.Errorf("status = %s, want grace_period", got)
	}
	// grace_ends_at = expires_at + 7 日历日（与 DB 内 expires 对齐，避免亚微秒精度抖动）。
	wantGrace := storedExpires.AddDate(0, 0, 7)
	if g := subGraceEndsAt(t, db, id); g == nil || !g.Equal(wantGrace) {
		t.Errorf("grace_ends_at = %v, want %v (expires+7d)", g, wantGrace)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_grace_period", id); n != 1 {
		t.Errorf("system audit count = %d, want 1 (actor_type=system, actor_id NULL)", n)
	}

	// 幂等：再调 → 已 grace_period，守卫不过 → (false,nil)，无新 audit。
	ok2, err := repo.TransitionSubscriptionToGrace(context.Background(), id, now, 7)
	if err != nil || ok2 {
		t.Fatalf("idempotent re-call = (%v,%v), want (false,nil)", ok2, err)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_grace_period", id); n != 1 {
		t.Errorf("audit after re-call = %d, want still 1", n)
	}
}

func TestTransitionSubscriptionToGrace_FrozenUntouched(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCommercialRepository(db)
	now := time.Now().UTC()

	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, brandID, "frozen", now.Add(-time.Hour), nil, 5)
	id := subIDForBrand(t, db, brandID)

	ok, err := repo.TransitionSubscriptionToGrace(context.Background(), id, now, 7)
	if err != nil || ok {
		t.Fatalf("frozen transition = (%v,%v), want (false,nil) 不动人工态", ok, err)
	}
	if got := subStatusOf(t, db, id); got != "frozen" {
		t.Errorf("frozen sub changed to %s, want untouched", got)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_grace_period", id); n != 0 {
		t.Errorf("frozen audit count = %d, want 0", n)
	}
}

// ---- ListSubscriptionsDueForRestricted ----

func TestListSubscriptionsDueForRestricted(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCommercialRepository(db)
	now := time.Now().UTC()
	tp := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	bHit, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bHit, "grace_period", now.Add(-240*time.Hour), tp(-time.Hour), 5) // grace 已过 → 命中
	idHit := subIDForBrand(t, db, bHit)

	bFuture, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bFuture, "grace_period", now.Add(-time.Hour), tp(time.Hour), 5) // grace 未来
	bNull, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bNull, "grace_period", now.Add(-time.Hour), nil, 5) // grace NULL（防御）
	bActive, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bActive, "active", now.Add(-time.Hour), nil, 5)

	ids, err := repo.ListSubscriptionsDueForRestricted(context.Background(), now)
	if err != nil {
		t.Fatalf("ListSubscriptionsDueForRestricted: %v", err)
	}
	if !int64InSlice(idHit, ids) {
		t.Errorf("grace+expired sub %d missing; got %v", idHit, ids)
	}
	for _, b := range []int64{bFuture, bNull, bActive} {
		if id := subIDForBrand(t, db, b); int64InSlice(id, ids) {
			t.Errorf("sub %d (brand %d) wrongly listed for restricted", id, b)
		}
	}
}

// ---- TransitionSubscriptionToRestricted ----

func TestTransitionSubscriptionToRestricted(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCommercialRepository(db)
	now := time.Now().UTC()
	tp := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, brandID, "grace_period", now.Add(-240*time.Hour), tp(-time.Hour), 5)
	id := subIDForBrand(t, db, brandID)

	ok, err := repo.TransitionSubscriptionToRestricted(context.Background(), id, now)
	if err != nil || !ok {
		t.Fatalf("TransitionSubscriptionToRestricted = (%v,%v), want (true,nil)", ok, err)
	}
	if got := subStatusOf(t, db, id); got != "restricted" {
		t.Errorf("status = %s, want restricted", got)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_restricted", id); n != 1 {
		t.Errorf("system audit count = %d, want 1", n)
	}

	// 幂等：再调 → restricted，守卫不过 → (false,nil)，无新 audit。
	ok2, err := repo.TransitionSubscriptionToRestricted(context.Background(), id, now)
	if err != nil || ok2 {
		t.Fatalf("idempotent re-call = (%v,%v), want (false,nil)", ok2, err)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_restricted", id); n != 1 {
		t.Errorf("audit after re-call = %d, want still 1", n)
	}

	// 守卫不过：active sub 调 restricted → (false,nil) 且不动。
	bActive, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, bActive, "active", now.Add(-time.Hour), nil, 5)
	idA := subIDForBrand(t, db, bActive)
	okA, err := repo.TransitionSubscriptionToRestricted(context.Background(), idA, now)
	if err != nil || okA {
		t.Fatalf("active→restricted = (%v,%v), want (false,nil)", okA, err)
	}
	if got := subStatusOf(t, db, idA); got != "active" {
		t.Errorf("active sub wrongly changed to %s", got)
	}
}
