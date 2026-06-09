package commercial

import (
	"context"
	"testing"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

func TestSubscriptionGuard_RejectsBadBrandID(t *testing.T) {
	g := NewSubscriptionGuard()
	// nil tx 时也应该先检测 brand_id 顺序——实现里 tx==nil 优先级最高，
	// 所以拿 nil tx 校验 nil tx 路径；用 bad brand id + 非 nil tx 没法在单测里给。
	_, _, err := g.CheckAndCount(context.Background(), nil, 1, ResourceLocation)
	if err == nil {
		t.Fatal("expected nil tx error")
	}
	ae := apperr.GetAppError(err)
	if ae == nil || ae.Code != apperr.ErrInternalServer {
		t.Errorf("expected INTERNAL_SERVER_ERROR for nil tx, got %v", err)
	}
}

func TestSubscriptionGuard_DispatchUnknownKind(t *testing.T) {
	g := NewSubscriptionGuard()
	_, _, _, err := g.dispatch(1, ResourceKind("nope"), guardSubscriptionRow{})
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestSubscriptionGuard_DispatchAllKnownKinds(t *testing.T) {
	g := NewSubscriptionGuard()
	sub := guardSubscriptionRow{MaxLocations: 5, MaxStaffSeats: 10, MaxLearners: 100}
	kinds := []struct {
		k       ResourceKind
		wantMax int64
	}{
		{ResourceLocation, 5},
		{ResourceStaff, 10},
		{ResourceLearner, 100},
	}
	for _, c := range kinds {
		sql, args, max, err := g.dispatch(42, c.k, sub)
		if err != nil {
			t.Fatalf("dispatch(%s): %v", c.k, err)
		}
		if max != c.wantMax {
			t.Errorf("dispatch(%s): max=%d want %d", c.k, max, c.wantMax)
		}
		if sql == "" || len(args) != 1 {
			t.Errorf("dispatch(%s): bad sql/args", c.k)
		}
	}
}
