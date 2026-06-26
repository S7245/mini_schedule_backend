// Package worker 后台 asynq 任务的入站适配器（Batch 15）。
// 仅做：解码任务 → 调 application 用例 → 记日志。不写业务状态机、不直接碰 GORM。
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/zkw/mini-schedule/backend/internal/application/sessionautomation"
)

// TaskSessionSweep asynq 任务类型：场次状态扫描推进。
const TaskSessionSweep = "session:sweep"

// NewSweepTask 构造一个 sweep 任务（无 payload）。Scheduler 按 cron enqueue 它。
func NewSweepTask() *asynq.Task {
	return asynq.NewTask(TaskSessionSweep, nil)
}

// SweepHandler 处理 session:sweep 任务。
type SweepHandler struct {
	svc *sessionautomation.Service
	log *slog.Logger
}

// NewSweepHandler 构造。
func NewSweepHandler(svc *sessionautomation.Service, log *slog.Logger) *SweepHandler {
	return &SweepHandler{svc: svc, log: log}
}

// Handle 处理一次 sweep。now 取处理时刻的真实墙钟（测试在 service 层注入可控时钟）。
// 返回 error 触发 asynq 重试（仅 systemic 失败；单场次失败已在 service 内隔离）。
func (h *SweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	sum, err := h.svc.RunSweep(ctx, time.Now().UTC())
	if err != nil {
		if h.log != nil {
			h.log.Error("session sweep failed", slog.Any("error", err))
		}
		return err
	}
	if h.log != nil {
		h.log.Info("session sweep done",
			slog.Int("started", sum.Started),
			slog.Int("ended", sum.Ended),
			slog.Int("skipped", sum.Skipped),
			slog.Int("failed", sum.Failed),
		)
	}
	return nil
}
