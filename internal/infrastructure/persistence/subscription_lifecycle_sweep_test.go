package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/application/subscriptionlifecycle"
)

// TestSubscriptionLifecycle_SelfHealOneRound 真实 PG 端到端：长期过期 active sub 在一轮 RunSweep
// 内 active→grace→restricted（证 phase1 的写对 phase2 扫描可见），两段 audit 都落，actor=system。
// 同时编译期证明 commercial.Repository 满足 subscriptionlifecycle.Repository 窄接口。
func TestSubscriptionLifecycle_SelfHealOneRound(t *testing.T) {
	db := newMigratedTestDB(t)
	svc := subscriptionlifecycle.NewService(NewCommercialRepository(db), 7, nil)
	now := time.Now().UTC()

	// active + expires_at 30 天前（+7d 仍 ≤ now）→ 期望一轮内 restricted。
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedSubState(t, db, brandID, "active", now.Add(-30*24*time.Hour), nil, 5)
	id := subIDForBrand(t, db, brandID)

	sum, err := svc.RunSweep(context.Background(), now)
	if err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	if sum.Graced != 1 || sum.Restricted != 1 {
		t.Errorf("summary = %+v, want Graced=1 Restricted=1 (一轮自愈)", sum)
	}
	if got := subStatusOf(t, db, id); got != "restricted" {
		t.Errorf("status = %s, want restricted", got)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_grace_period", id); n != 1 {
		t.Errorf("grace audit = %d, want 1", n)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_restricted", id); n != 1 {
		t.Errorf("restricted audit = %d, want 1", n)
	}

	// 幂等：再跑一轮，状态不变、计数全 0、无新 audit。
	sum2, err := svc.RunSweep(context.Background(), now)
	if err != nil {
		t.Fatalf("RunSweep 2: %v", err)
	}
	if sum2.Graced != 0 || sum2.Restricted != 0 || sum2.Skipped != 0 || sum2.Failed != 0 {
		t.Errorf("second sweep = %+v, want all-zero (已翻 sub 掉出扫描)", sum2)
	}
	if got := subStatusOf(t, db, id); got != "restricted" {
		t.Errorf("status after re-sweep = %s, want restricted", got)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_grace_period", id); n != 1 {
		t.Errorf("grace audit after re-sweep = %d, want still 1", n)
	}
	if n := countSystemSubAudit(t, db, "brand_subscription.auto_restricted", id); n != 1 {
		t.Errorf("restricted audit after re-sweep = %d, want still 1", n)
	}
}

// TestSubscriptionLifecycle_Sweep_GraceThenLaterRestricted 分两轮：先到期进 grace（grace_ends_at 未来），
// 宽限期内不受限；待 grace_ends_at 过后再 sweep 才 restricted。证宽限窗真实生效。
func TestSubscriptionLifecycle_Sweep_GraceThenLaterRestricted(t *testing.T) {
	db := newMigratedTestDB(t)
	svc := subscriptionlifecycle.NewService(NewCommercialRepository(db), 7, nil)

	brandID, _ := seedBrandWithSystemRoles(t, db)
	// expires 2 天前 → 第一轮（now=t0）进 grace，grace_ends_at = expires+7d ≈ 5 天后（未来）。
	t0 := time.Now().UTC()
	seedSubState(t, db, brandID, "active", t0.Add(-2*24*time.Hour), nil, 5)
	id := subIDForBrand(t, db, brandID)

	sum, err := svc.RunSweep(context.Background(), t0)
	if err != nil {
		t.Fatalf("RunSweep t0: %v", err)
	}
	if sum.Graced != 1 || sum.Restricted != 0 {
		t.Errorf("t0 summary = %+v, want Graced=1 Restricted=0 (宽限窗未过)", sum)
	}
	if got := subStatusOf(t, db, id); got != "grace_period" {
		t.Errorf("t0 status = %s, want grace_period", got)
	}

	// 推进到 grace_ends_at 之后（t0 + 10 天）→ 第二轮 restricted。
	t1 := t0.Add(10 * 24 * time.Hour)
	sum2, err := svc.RunSweep(context.Background(), t1)
	if err != nil {
		t.Fatalf("RunSweep t1: %v", err)
	}
	if sum2.Graced != 0 || sum2.Restricted != 1 {
		t.Errorf("t1 summary = %+v, want Graced=0 Restricted=1", sum2)
	}
	if got := subStatusOf(t, db, id); got != "restricted" {
		t.Errorf("t1 status = %s, want restricted", got)
	}
}
