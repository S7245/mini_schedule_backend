// Package sessionautomation 场次状态自动化用例（Batch 15）。
//
// 无 RBAC（系统执行，非 brand_user）；由 cmd/worker 的 asynq sweep handler 周期调用。
// 复用 RBAC-free 的 booking.Repository（EndSessionSystem / 扫描方法），不经 brand
// booking.Service 的权限门（系统无 attendance.mark 等品牌权限码）。
package sessionautomation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// Service 场次状态自动化服务。
type Service struct {
	repo booking.Repository
	log  *slog.Logger
}

// NewService 构造。log 可为 nil（仅影响单场次失败日志）。
func NewService(repo booking.Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Summary 一轮 sweep 的处理统计（供 handler 记日志）。
type Summary struct {
	Started int // scheduled→in_progress（到点进行中）
	Ended   int // →completed（到点结束，产 pending_no_show）
	Skipped int // 并发已终态，EndSession 返 SESSION_NOT_ENDABLE（幂等空操作）
	Failed  int // 单场次 EndSession 非预期报错（已 log，下一轮自愈）
}

// RunSweep 执行一轮场次状态推进（Batch 15）：
//  1. scheduled→in_progress（到点进行中）批量、幂等、无 audit；
//  2. 到点未结束场次逐个系统版 EndSession（每场次独立 tx，失败隔离 + 幂等）。
//
// now 由调用方注入：worker 传 time.Now().UTC()，测试注入固定时钟。
// systemic 失败（扫描查询出错）返 error 触发 asynq 重试；单场次失败仅 log+continue。
func (s *Service) RunSweep(ctx context.Context, now time.Time) (Summary, error) {
	var sum Summary

	started, err := s.repo.MarkSessionsInProgress(ctx, now)
	if err != nil {
		return sum, err
	}
	sum.Started = int(started)

	ids, err := s.repo.ListDueSessionIDs(ctx, now)
	if err != nil {
		return sum, err
	}
	for _, id := range ids {
		if _, err := s.repo.EndSessionSystem(ctx, id); err != nil {
			// 并发已被结束/取消（NotEndable）或已删除（NotFound）：幂等空操作，按 skipped 计，
			// 不算失败（这些是 List→End 之间的良性竞态，不应刷 error 日志或抬高 failed 指标）。
			var ae *apperr.AppError
			if errors.As(err, &ae) && (ae.Code == apperr.ErrSessionNotEndable || ae.Code == apperr.ErrSessionNotFound) {
				sum.Skipped++
				continue
			}
			sum.Failed++
			if s.log != nil {
				s.log.Error("auto end session failed",
					slog.Int64("session_id", id), slog.Any("error", err))
			}
			continue
		}
		sum.Ended++
	}
	return sum, nil
}
