// Package subscriptionlifecycle 订阅生命周期自动化用例（Batch 16）。
//
// 无 RBAC（系统执行，非 brand_user）；由 cmd/worker 的 asynq subscription:sweep handler
// 周期调用。复用 commercial.Repository 的系统转换方法（ListSubscriptionsDueFor* /
// TransitionSubscriptionTo*），不经平台 admin 订阅状态机的权限门。镜像 Batch 15
// sessionautomation：扫描 + 逐 sub 系统事务 + 失败隔离 + 幂等 + Summary。
package subscriptionlifecycle

import (
	"context"
	"log/slog"
	"time"
)

// DefaultGraceDays §1334 默认宽限期（可系统配置）。NewService 在 graceDays<=0 时兜底用它，
// cmd/worker 也引用它做启动日志的有效值兜底（单一真源，避免字面量漂移）。
const DefaultGraceDays = 7

// Repository 订阅生命周期所需的窄接口（commercial.Repository 结构上满足）。
type Repository interface {
	ListSubscriptionsDueForGrace(ctx context.Context, now time.Time) ([]int64, error)
	TransitionSubscriptionToGrace(ctx context.Context, id int64, now time.Time, graceDays int) (bool, error)
	ListSubscriptionsDueForRestricted(ctx context.Context, now time.Time) ([]int64, error)
	TransitionSubscriptionToRestricted(ctx context.Context, id int64, now time.Time) (bool, error)
}

// Service 订阅生命周期自动化服务。
type Service struct {
	repo      Repository
	graceDays int
	log       *slog.Logger
}

// NewService 构造。graceDays<=0 兜底为 7。log 可为 nil（仅影响单 sub 失败日志）。
func NewService(repo Repository, graceDays int, log *slog.Logger) *Service {
	if graceDays <= 0 {
		graceDays = DefaultGraceDays
	}
	return &Service{repo: repo, graceDays: graceDays, log: log}
}

// Summary 一轮 sweep 的处理统计（供 handler 记日志）。
type Summary struct {
	Graced     int // active→grace_period（到期进宽限期）
	Restricted int // grace_period→restricted（宽限期满受限）
	Skipped    int // 守卫不过（并发已被平台改动），(false,nil) 良性空操作
	Failed     int // 单 sub 转换非预期报错（已 log，下一 tick 自愈）
}

// RunSweep 执行一轮订阅生命周期推进（Batch 16）：
//  1. phase1 active→grace_period（expires_at 到期）；
//  2. phase2 grace_period→restricted（grace_ends_at 宽限期满）。
//
// phase1 先于 phase2：长期过期 sub（expires_at 与 expires_at+graceDays 均 ≤ now）在同一轮内
// active→grace→restricted 自愈（phase1 置 grace_ends_at≤now，phase2 立即扫到）。
//
// now 由调用方注入：worker 传 time.Now().UTC()，测试注入固定时钟。
// systemic 失败（List 查询出错）返 error 触发 asynq 重试；单 sub 失败仅 log+continue（下一 tick 自愈）。
func (s *Service) RunSweep(ctx context.Context, now time.Time) (Summary, error) {
	var sum Summary

	graceIDs, err := s.repo.ListSubscriptionsDueForGrace(ctx, now)
	if err != nil {
		return sum, err
	}
	for _, id := range graceIDs {
		ok, err := s.repo.TransitionSubscriptionToGrace(ctx, id, now, s.graceDays)
		if err != nil {
			sum.Failed++
			if s.log != nil {
				s.log.Error("auto grace transition failed",
					slog.Int64("subscription_id", id), slog.Any("error", err))
			}
			continue
		}
		if ok {
			sum.Graced++
		} else {
			sum.Skipped++
		}
	}

	restrictedIDs, err := s.repo.ListSubscriptionsDueForRestricted(ctx, now)
	if err != nil {
		return sum, err
	}
	for _, id := range restrictedIDs {
		ok, err := s.repo.TransitionSubscriptionToRestricted(ctx, id, now)
		if err != nil {
			sum.Failed++
			if s.log != nil {
				s.log.Error("auto restricted transition failed",
					slog.Int64("subscription_id", id), slog.Any("error", err))
			}
			continue
		}
		if ok {
			sum.Restricted++
		} else {
			sum.Skipped++
		}
	}
	return sum, nil
}
