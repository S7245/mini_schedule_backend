package persistence

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// seedSubState 给 brand 造一条任意状态/时间窗的订阅（含 saas_plan），供 SubscriptionGuard
// 六态走查。starts_at 恒取 expires_at - 30d（满足 CHECK expires_at > starts_at）。
// graceEndsAt 为 nil 时写 NULL。maxLocations 作 Location 配额上限。
func seedSubState(t *testing.T, db *gorm.DB, brandID int64, status string, expiresAt time.Time, graceEndsAt *time.Time, maxLocations int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO saas_plans (name, max_locations, max_staff_seats, max_learners) VALUES (?, ?, 100, 100)`,
		"测试套餐", maxLocations,
	).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	var planID int64
	if err := db.Raw(`SELECT id FROM saas_plans ORDER BY id DESC LIMIT 1`).Scan(&planID).Error; err != nil {
		t.Fatalf("read plan id: %v", err)
	}
	startsAt := expiresAt.AddDate(0, 0, -30)
	if err := db.Exec(
		`INSERT INTO brand_subscriptions
		   (brand_id, plan_id, billing_cycle, status, starts_at, expires_at, grace_ends_at, max_locations, max_staff_seats, max_learners)
		 VALUES (?, ?, 'monthly', ?, ?, ?, ?, ?, 100, 100)`,
		brandID, planID, status, startsAt, expiresAt, graceEndsAt, maxLocations,
	).Error; err != nil {
		t.Fatalf("seed subscription(%s): %v", status, err)
	}
}

func guardCheck(t *testing.T, db *gorm.DB, g *commercial.SubscriptionGuard, brandID int64) error {
	t.Helper()
	var gotErr error
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, _, gotErr = g.CheckAndCount(context.Background(), tx, brandID, commercial.ResourceLocation)
		return nil
	}); err != nil {
		t.Fatalf("tx: %v", err)
	}
	return gotErr
}

// TestSubscriptionGuard_StatusWalk 六态走查（Batch 16 放宽 guard：active+grace_period 视同可用）。
// 时间门提供纵深防御：active/grace 过期仍按读时硬限拦。
func TestSubscriptionGuard_StatusWalk(t *testing.T) {
	db := newMigratedTestDB(t)
	g := commercial.NewSubscriptionGuard()
	now := time.Now().UTC()
	tp := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	cases := []struct {
		name    string
		status  string
		expires time.Time
		grace   *time.Time
		wantErr apperr.ErrorCode // "" = 放行
	}{
		{"active_future", "active", now.Add(24 * time.Hour), nil, ""},
		{"active_expired", "active", now.Add(-24 * time.Hour), nil, apperr.ErrSubscriptionRestricted},
		{"grace_in_window", "grace_period", now.Add(-24 * time.Hour), tp(24 * time.Hour), ""},
		{"grace_out_window", "grace_period", now.Add(-240 * time.Hour), tp(-24 * time.Hour), apperr.ErrSubscriptionRestricted},
		{"restricted", "restricted", now.Add(-240 * time.Hour), tp(-24 * time.Hour), apperr.ErrSubscriptionRestricted},
		{"expired", "expired", now.Add(-240 * time.Hour), nil, apperr.ErrSubscriptionRestricted},
		{"frozen", "frozen", now.Add(-240 * time.Hour), nil, apperr.ErrSubscriptionRestricted},
		{"cancelled", "cancelled", now.Add(-240 * time.Hour), nil, apperr.ErrSubscriptionRestricted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			brandID, _ := seedBrandWithSystemRoles(t, db)
			seedSubState(t, db, brandID, c.status, c.expires, c.grace, 5)
			gotErr := guardCheck(t, db, g, brandID)
			if c.wantErr == "" {
				if gotErr != nil {
					t.Fatalf("%s: expected pass, got %v", c.name, gotErr)
				}
				return
			}
			assertAppCode(t, gotErr, c.wantErr)
		})
	}
}

// TestSubscriptionGuard_OverQuota 放宽后配额门仍生效：active 与 grace_period 满员都返 QUOTA_EXCEEDED
// （而非因 grace 不被识别而误判 SUBSCRIPTION_RESTRICTED）。
func TestSubscriptionGuard_OverQuota(t *testing.T) {
	db := newMigratedTestDB(t)
	g := commercial.NewSubscriptionGuard()
	now := time.Now().UTC()
	tp := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	cases := []struct {
		name    string
		status  string
		expires time.Time
		grace   *time.Time
	}{
		{"active_at_cap", "active", now.Add(24 * time.Hour), nil},
		{"grace_at_cap", "grace_period", now.Add(-24 * time.Hour), tp(24 * time.Hour)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			brandID, _ := seedBrandWithSystemRoles(t, db)
			seedSubState(t, db, brandID, c.status, c.expires, c.grace, 1) // max_locations=1
			seedLocation(t, db, brandID, "讯美广场")                          // 已用 1 = 满
			gotErr := guardCheck(t, db, g, brandID)
			assertAppCode(t, gotErr, apperr.ErrQuotaExceeded)
		})
	}
}
