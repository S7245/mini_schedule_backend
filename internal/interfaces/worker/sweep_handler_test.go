package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/application/sessionautomation"
	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
)

// stubRepo 仅覆盖 sweep 用到的方法（其余继承 nil 接口，不被调用）。
type stubRepo struct {
	booking.Repository
	markErr error
}

func (s stubRepo) MarkSessionsInProgress(context.Context, time.Time) (int64, error) {
	return 0, s.markErr
}
func (s stubRepo) ListDueSessionIDs(context.Context, time.Time) ([]int64, error) { return nil, nil }

func TestSweepHandler_Handle_OK(t *testing.T) {
	h := NewSweepHandler(sessionautomation.NewService(stubRepo{}, nil), nil)
	if err := h.Handle(context.Background(), NewSweepTask()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestSweepHandler_Handle_PropagatesSystemicError(t *testing.T) {
	h := NewSweepHandler(sessionautomation.NewService(stubRepo{markErr: errors.New("db down")}, nil), nil)
	if err := h.Handle(context.Background(), NewSweepTask()); err == nil {
		t.Fatal("systemic 失败应返 error 触发 asynq 重试")
	}
}
