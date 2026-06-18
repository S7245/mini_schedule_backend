package entitlement

import (
	"testing"
	"time"
)

func TestSettleStatus(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)
	ip := func(n int) *int { return &n }

	cases := []struct {
		name      string
		current   Status
		expires   time.Time
		total     *int
		remaining *int
		want      Status
	}{
		{"frozen passthrough even if expired", StatusFrozen, past, ip(10), ip(0), StatusFrozen},
		{"cancelled passthrough even if active-ish", StatusCancelled, future, ip(10), ip(5), StatusCancelled},
		{"active normal", StatusActive, future, ip(10), ip(5), StatusActive},
		{"active past expiry -> expired (even with credits)", StatusActive, past, ip(10), ip(5), StatusExpired},
		{"active remaining 0 -> depleted", StatusActive, future, ip(10), ip(0), StatusDepleted},
		{"expiry precedence over depleted", StatusActive, past, ip(10), ip(0), StatusExpired},
		{"unlimited (nil total) future -> active", StatusActive, future, nil, nil, StatusActive},
		{"unlimited past expiry -> expired", StatusActive, past, nil, nil, StatusExpired},
		{"expires exactly now -> expired (<=)", StatusActive, now, ip(10), ip(5), StatusExpired},
		{"recompute depleted->active when credits added back", StatusDepleted, future, ip(10), ip(3), StatusActive},
		{"recompute expired->active when not yet expired", StatusExpired, future, ip(10), ip(5), StatusActive},
		{"depleted stays depleted while remaining 0", StatusDepleted, future, ip(5), ip(0), StatusDepleted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SettleStatus(c.current, c.expires, c.total, c.remaining, now)
			if got != c.want {
				t.Fatalf("SettleStatus(%s, exp=%v, total=%v, rem=%v) = %s, want %s",
					c.current, c.expires, c.total, c.remaining, got, c.want)
			}
		})
	}
}
