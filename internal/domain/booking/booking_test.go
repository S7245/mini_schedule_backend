package booking

import (
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
)

func ptrInt(v int) *int    { return &v }
func ptrBool(v bool) *bool { return &v }

func TestIsValidStatusAndMode(t *testing.T) {
	for _, s := range []string{"booked", "cancelled", "attended", "pending_no_show", "no_show"} {
		if !IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q)=false, want true", s)
		}
	}
	if IsValidStatus("bogus") {
		t.Error("IsValidStatus(bogus)=true, want false")
	}
	for _, m := range []string{"auto", "manual", "none"} {
		if !IsValidEntitlementMode(m) {
			t.Errorf("IsValidEntitlementMode(%q)=false, want true", m)
		}
	}
	if IsValidEntitlementMode("self") {
		t.Error("IsValidEntitlementMode(self)=true, want false")
	}
}

func TestResolveEffectivePolicy(t *testing.T) {
	base := Policy{
		BookAheadMinMinutes:   30,
		BookAheadMaxMinutes:   ptrInt(10080),
		CancelDeadlineMinutes: 60,
		ReleaseOnCancel:       true,
		AllowWaitlist:         true,
		WaitlistLimit:         5,
	}

	// nil override：base 透传，AllowCancel 默认 true。
	eff := ResolveEffectivePolicy(base, nil)
	if !eff.AllowCancel || eff.CancelDeadlineMinutes != 60 || !eff.ReleaseOnCancel {
		t.Fatalf("nil override mismatch: %+v", eff)
	}

	// 稀疏 override：仅覆盖给定字段，其余继承 base。
	eff = ResolveEffectivePolicy(base, &PolicyOverride{
		AllowCancel:           ptrBool(false),
		CancelDeadlineMinutes: ptrInt(120),
		ReleaseOnCancel:       ptrBool(false),
	})
	if eff.AllowCancel {
		t.Error("AllowCancel override=false not applied")
	}
	if eff.CancelDeadlineMinutes != 120 {
		t.Errorf("CancelDeadlineMinutes=%d, want 120", eff.CancelDeadlineMinutes)
	}
	if eff.ReleaseOnCancel {
		t.Error("ReleaseOnCancel override=false not applied")
	}
	// 未覆盖字段继承 base。
	if eff.WaitlistLimit != 5 || !eff.AllowWaitlist || eff.BookAheadMinMinutes != 30 {
		t.Errorf("inherited fields mismatch: %+v", eff)
	}
}

func TestWithinBookingWindow(t *testing.T) {
	starts := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	p := EffectivePolicy{Policy: Policy{BookAheadMinMinutes: 30, BookAheadMaxMinutes: ptrInt(7 * 24 * 60)}}

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"刚好窗口内", starts.Add(-2 * time.Hour), true},
		{"太晚-不足30分钟提前", starts.Add(-20 * time.Minute), false},
		{"恰好min边界", starts.Add(-30 * time.Minute), true},
		{"太早-超7天", starts.Add(-8 * 24 * time.Hour), false},
		{"恰好max边界", starts.Add(-7 * 24 * time.Hour), true},
		{"开始后", starts.Add(1 * time.Minute), false},
	}
	for _, c := range cases {
		if got := WithinBookingWindow(c.now, starts, p); got != c.want {
			t.Errorf("%s: WithinBookingWindow=%v, want %v", c.name, got, c.want)
		}
	}

	// max=nil 不限提前。
	pNoMax := EffectivePolicy{Policy: Policy{BookAheadMinMinutes: 0}}
	if !WithinBookingWindow(starts.Add(-365*24*time.Hour), starts, pNoMax) {
		t.Error("max=nil 应允许任意提前")
	}
}

func TestCancelDeadlinePassed(t *testing.T) {
	starts := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	p := EffectivePolicy{Policy: Policy{CancelDeadlineMinutes: 120}}
	if CancelDeadlinePassed(starts.Add(-3*time.Hour), starts, p) {
		t.Error("3小时前取消不应过截止")
	}
	if !CancelDeadlinePassed(starts.Add(-1*time.Hour), starts, p) {
		t.Error("1小时前(截止2小时)应已过截止")
	}
	// deadline=0：开始前都可取消。
	p0 := EffectivePolicy{Policy: Policy{CancelDeadlineMinutes: 0}}
	if CancelDeadlinePassed(starts.Add(-1*time.Minute), starts, p0) {
		t.Error("deadline=0 开始前不应过截止")
	}
}

func TestSortCandidates_Priority(t *testing.T) {
	early := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// 指定课程权益（CourseSpecific）应排第一，哪怕过期更晚。
	cands := []EntitlementCandidate{
		{EntitlementID: 1, ProductType: entitlement.ProductClassPack, CourseSpecific: false, ExpiresAt: early},
		{EntitlementID: 2, ProductType: entitlement.ProductMembershipCard, CourseSpecific: true, ExpiresAt: late},
	}
	got, ok := SelectAuto(cands)
	if !ok || got.EntitlementID != 2 {
		t.Fatalf("指定课程权益应优先, got %+v ok=%v", got, ok)
	}

	// 同 specificity：最早过期优先（规则2 先于规则3 类型）。
	cands = []EntitlementCandidate{
		{EntitlementID: 1, ProductType: entitlement.ProductMembershipCard, ExpiresAt: early},
		{EntitlementID: 2, ProductType: entitlement.ProductClassPack, ExpiresAt: late},
	}
	got, _ = SelectAuto(cands)
	if got.EntitlementID != 1 {
		t.Errorf("最早过期应优先于类型, got id=%d", got.EntitlementID)
	}

	// 同 specificity 同过期：课时包先于会员卡（规则3）。
	cands = []EntitlementCandidate{
		{EntitlementID: 1, ProductType: entitlement.ProductMembershipCard, ExpiresAt: early},
		{EntitlementID: 2, ProductType: entitlement.ProductClassPack, ExpiresAt: early},
	}
	got, _ = SelectAuto(cands)
	if got.EntitlementID != 2 {
		t.Errorf("课时包应先于会员卡, got id=%d", got.EntitlementID)
	}

	// 空候选 ok=false。
	if _, ok := SelectAuto(nil); ok {
		t.Error("空候选应 ok=false")
	}
}
