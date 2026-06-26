package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/zkw/mini-schedule/backend/internal/application/subscriptionlifecycle"
)

// subStubRepo 实现 subscriptionlifecycle.Repository，按需在 phase1 List 注入 systemic 错误。
type subStubRepo struct {
	listErr error
}

func (s subStubRepo) ListSubscriptionsDueForGrace(context.Context, time.Time) ([]int64, error) {
	return nil, s.listErr
}
func (s subStubRepo) TransitionSubscriptionToGrace(context.Context, int64, time.Time, int) (bool, error) {
	return false, nil
}
func (s subStubRepo) ListSubscriptionsDueForRestricted(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (s subStubRepo) TransitionSubscriptionToRestricted(context.Context, int64, time.Time) (bool, error) {
	return false, nil
}

func TestSubscriptionSweepTask_TypeDistinct(t *testing.T) {
	if TaskSubscriptionSweep == TaskSessionSweep {
		t.Fatal("subscription/session task types must differ")
	}
	if got := NewSubscriptionSweepTask().Type(); got != TaskSubscriptionSweep {
		t.Errorf("task type = %q, want %q", got, TaskSubscriptionSweep)
	}
}

func TestMux_RegistersBothSweepsNoConflict(t *testing.T) {
	mux := asynq.NewServeMux()
	// 同一 mux 注册两类任务，类型不同 → HandleFunc 不应 panic（路由不冲突）。
	mux.HandleFunc(TaskSessionSweep, func(context.Context, *asynq.Task) error { return nil })
	mux.HandleFunc(TaskSubscriptionSweep, func(context.Context, *asynq.Task) error { return nil })
}

func TestSubscriptionSweepHandler_Handle_OK(t *testing.T) {
	h := NewSubscriptionSweepHandler(subscriptionlifecycle.NewService(subStubRepo{}, 7, nil), nil)
	if err := h.Handle(context.Background(), NewSubscriptionSweepTask()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestSubscriptionSweepHandler_Handle_PropagatesSystemicError(t *testing.T) {
	h := NewSubscriptionSweepHandler(subscriptionlifecycle.NewService(subStubRepo{listErr: errors.New("db down")}, 7, nil), nil)
	if err := h.Handle(context.Background(), NewSubscriptionSweepTask()); err == nil {
		t.Fatal("systemic 失败应返 error 触发 asynq 重试")
	}
}
