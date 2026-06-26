package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/zkw/mini-schedule/backend/internal/application/subscriptionlifecycle"
)

// TaskSubscriptionSweep asynq 任务类型：订阅生命周期扫描推进（Batch 16）。
const TaskSubscriptionSweep = "subscription:sweep"

// NewSubscriptionSweepTask 构造一个订阅 sweep 任务（无 payload）。Scheduler 按 cron enqueue 它。
func NewSubscriptionSweepTask() *asynq.Task {
	return asynq.NewTask(TaskSubscriptionSweep, nil)
}

// SubscriptionSweepHandler 处理 subscription:sweep 任务。
type SubscriptionSweepHandler struct {
	svc *subscriptionlifecycle.Service
	log *slog.Logger
}

// NewSubscriptionSweepHandler 构造。
func NewSubscriptionSweepHandler(svc *subscriptionlifecycle.Service, log *slog.Logger) *SubscriptionSweepHandler {
	return &SubscriptionSweepHandler{svc: svc, log: log}
}

// Handle 处理一次订阅 sweep。now 取处理时刻真实墙钟（测试在 service 层注入可控时钟）。
// 返回 error 触发 asynq 重试（仅 systemic 失败；单 sub 失败已在 service 内隔离）。
func (h *SubscriptionSweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	sum, err := h.svc.RunSweep(ctx, time.Now().UTC())
	if err != nil {
		if h.log != nil {
			h.log.Error("subscription sweep failed", slog.Any("error", err))
		}
		return err
	}
	if h.log != nil {
		h.log.Info("subscription sweep done",
			slog.Int("graced", sum.Graced),
			slog.Int("restricted", sum.Restricted),
			slog.Int("skipped", sum.Skipped),
			slog.Int("failed", sum.Failed),
		)
	}
	return nil
}
