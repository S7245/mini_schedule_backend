// Package report 品牌基础运营看板应用服务（Batch 17，§15 品牌看板）。
//
// 权限：report.view_basic（已在 000003 seed → owner/admin/course_operator/location_manager）。
// data_scope：owner/admin/course_operator → 全品牌；location_manager → assigned locations
// （镜像 13c booking.scopeFilterIDs）。纯读、无写、无 audit。
package report

import (
	"context"
	"time"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainreport "github.com/zkw/mini-schedule/backend/internal/domain/report"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// permReportViewBasic 基础报表查看权限码（000003 已 seed）。
const permReportViewBasic = "report.view_basic"

// PermissionChecker service 需要的最小 Checker 面（Require + Resolve），镜像 booking.PermissionChecker。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 报表应用服务。
type Service struct {
	repo    domainreport.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope（兼容 bootstrap）。
func NewService(repo domainreport.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

// OverviewInput 品牌看板查询入参（handler 透传原始 query）。
type OverviewInput struct {
	BrandID    int64
	ActorID    int64
	Preset     string // today / this_week / this_month（默认）/ custom
	FromDate   string // custom 必填，YYYY-MM-DD
	ToDate     string // custom 必填，YYYY-MM-DD
	LocationID *int64 // 可选，单门店；须 ∈ scope
}

// GetBrandOverview 查品牌运营看板：权限门 → data_scope → 解析时间窗 → 聚合。
func (s *Service) GetBrandOverview(ctx context.Context, in OverviewInput) (*domainreport.BrandOverview, error) {
	if s.checker != nil {
		if err := s.checker.Require(ctx, in.BrandID, in.ActorID, permReportViewBasic); err != nil {
			return nil, err
		}
	}
	scopeIDs, err := s.scopeFilterIDs(ctx, in.BrandID, in.ActorID)
	if err != nil {
		return nil, err
	}
	if in.LocationID != nil {
		if err := guardLocationInScope(scopeIDs, *in.LocationID); err != nil {
			return nil, err
		}
	}
	from, to, err := resolveWindow(in.Preset, in.FromDate, in.ToDate, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return s.repo.BrandOverviewCounts(ctx, domainreport.ReportQuery{
		BrandID:          in.BrandID,
		ScopeLocationIDs: scopeIDs,
		LocationID:       in.LocationID,
		From:             from,
		To:               to,
	})
}

// scopeFilterIDs 把 actor 的 data_scope 转为 location id 过滤集。nil = 全品牌；空切片 = 拒绝所有。
func (s *Service) scopeFilterIDs(ctx context.Context, brandID, actorID int64) ([]int64, error) {
	if s.checker == nil {
		return nil, nil
	}
	_, scope, err := s.checker.Resolve(ctx, brandID, actorID)
	if err != nil {
		return nil, err
	}
	switch scope.Kind {
	case domainrbac.DataScopeAllBrand:
		return nil, nil
	case domainrbac.DataScopeAssignedLocations:
		if len(scope.LocationIDs) == 0 {
			return []int64{}, nil
		}
		return scope.LocationIDs, nil
	default:
		return []int64{}, nil
	}
}

// guardLocationInScope scope 受限时目标门店须在 scope 内，否则 403。nil scope = 全品牌放行。
func guardLocationInScope(scopeIDs []int64, locationID int64) error {
	if scopeIDs == nil {
		return nil
	}
	for _, id := range scopeIDs {
		if id == locationID {
			return nil
		}
	}
	return apperr.ErrForbiddenF("无权查看该门店报表")
}

// resolveWindow 把 preset / custom 区间解析为 [from, to) UTC（窗口边界 UTC，per-brand 时区留 FR）。
func resolveWindow(preset, fromDate, toDate string, now time.Time) (time.Time, time.Time, error) {
	dayStart := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	switch preset {
	case "", "this_month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	case "today":
		start := dayStart(now)
		return start, start.AddDate(0, 0, 1), nil
	case "this_week": // 周一为周首
		offset := (int(now.Weekday()) + 6) % 7
		start := dayStart(now).AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 7), nil
	case "custom":
		if fromDate == "" || toDate == "" {
			return time.Time{}, time.Time{}, apperr.ErrBadRequest("自定义区间须传 from、to 日期")
		}
		from, err := time.ParseInLocation("2006-01-02", fromDate, time.UTC)
		if err != nil {
			return time.Time{}, time.Time{}, apperr.ErrBadRequest("from 日期格式应为 YYYY-MM-DD")
		}
		to, err := time.ParseInLocation("2006-01-02", toDate, time.UTC)
		if err != nil {
			return time.Time{}, time.Time{}, apperr.ErrBadRequest("to 日期格式应为 YYYY-MM-DD")
		}
		if to.Before(from) {
			return time.Time{}, time.Time{}, apperr.ErrBadRequest("to 不能早于 from")
		}
		return from, to.AddDate(0, 0, 1), nil // 右开 +1 天，含 to 当天
	default:
		return time.Time{}, time.Time{}, apperr.ErrBadRequest("无效的时间范围")
	}
}
