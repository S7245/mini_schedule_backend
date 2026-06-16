// Package recurringschedule 循环排课应用服务（Batch 12b）。
//
// 负责入参校验 + 按时区（Asia/Shanghai，固定 +08:00）展开 occurrence 列表；生成的事务/冲突
// 跳过逻辑在 persistence 层。
package recurringschedule

import (
	"context"
	"fmt"
	"sort"
	"time"

	domainsession "github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainrec "github.com/zkw/mini-schedule/backend/internal/domain/recurringschedule"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// cstZone 生成时区，固定 +08:00（中国无 DST；避免依赖容器 tzdata）。
var cstZone = time.FixedZone("Asia/Shanghai", 8*3600)

const (
	maxHorizonDays = 26 * 7 // 最长 26 周跨度
	maxOccurrences = 200    // 单次生成硬上限
	dateLayout     = "2006-01-02"
	timeLayout     = "15:04"
)

// PermissionChecker 是 service 需要的最小 Checker 面（Require + Resolve）。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 循环排课应用服务。
type Service struct {
	repo    domainrec.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope。
func NewService(repo domainrec.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

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

func (s *Service) guardLocationInScope(ctx context.Context, brandID, actorID, locationID int64) error {
	ids, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	if ids == nil {
		return nil
	}
	for _, lid := range ids {
		if lid == locationID {
			return nil
		}
	}
	return apperr.NewAppError(apperr.ErrRecurringNotFound, "循环排课不存在", 404)
}

// GenerateInput 生成入参（原始请求字段）。
type GenerateInput struct {
	BrandID             int64
	ActorID             int64
	CourseID            int64
	LocationID          int64
	LocationResourceID  *int64
	InstructorProfileID int64
	Weekdays            []int
	StartDate           string // YYYY-MM-DD
	EndDate             string // YYYY-MM-DD（可空）
	RepeatWeeks         *int
	StartTime           string // HH:mm
	DurationMin         int
	Capacity            int
}

// ListInput 列表查询。
type ListInput struct {
	BrandID    int64
	ActorID    int64
	LocationID int64
	Status     string
	Page       int
	PageSize   int
}

// Generate 校验入参 + 展开 occurrence → 调 repo 生成。
func (s *Service) Generate(ctx context.Context, in GenerateInput) (*domainrec.GenerateResult, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "session.create"); err != nil {
		return nil, err
	}
	if in.CourseID <= 0 || in.LocationID <= 0 || in.InstructorProfileID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "课程 / 门店 / 教练不能为空", 400)
	}
	if in.DurationMin <= 0 {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "时长必须大于 0", 400)
	}
	if in.LocationResourceID != nil && *in.LocationResourceID <= 0 {
		in.LocationResourceID = nil
	}

	weekdays, err := normalizeWeekdays(in.Weekdays)
	if err != nil {
		return nil, err
	}
	startDate, err := time.ParseInLocation(dateLayout, in.StartDate, cstZone)
	if err != nil {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "开始日期格式错误（YYYY-MM-DD）", 400)
	}
	hh, mm, err := parseHHMM(in.StartTime)
	if err != nil {
		return nil, err
	}
	endDate, err := s.resolveEndDate(in, startDate)
	if err != nil {
		return nil, err
	}

	// start_date 不早于今天（按本地时区的当天 0 点）。
	todayLocal := time.Now().In(cstZone)
	today := time.Date(todayLocal.Year(), todayLocal.Month(), todayLocal.Day(), 0, 0, 0, 0, cstZone)
	if startDate.Before(today) {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "开始日期不能早于今天", 400)
	}
	if endDate.Sub(startDate) > maxHorizonDays*24*time.Hour {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "排课区间过长（最长 26 周）", 400)
	}

	// data_scope：只能在 scope 内门店排课。
	if err := s.guardLocationInScope(ctx, in.BrandID, in.ActorID, in.LocationID); err != nil {
		return nil, err
	}

	occ := buildOccurrences(startDate, endDate, weekdays, hh, mm, in.DurationMin, time.Now())
	if len(occ) == 0 {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "所选区间内没有匹配的排课日期", 400)
	}
	if len(occ) > maxOccurrences {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid,
			fmt.Sprintf("预计生成 %d 节，超过单次上限 %d 节，请缩短区间或减少星期", len(occ), maxOccurrences), 400)
	}

	return s.repo.Generate(ctx, domainrec.GenerateInput{
		BrandID:             in.BrandID,
		ActorID:             in.ActorID,
		CourseID:            in.CourseID,
		LocationID:          in.LocationID,
		LocationResourceID:  in.LocationResourceID,
		InstructorProfileID: in.InstructorProfileID,
		Weekdays:            weekdays,
		StartDate:           in.StartDate,
		EndDate:             endDate.Format(dateLayout),
		RepeatWeeks:         in.RepeatWeeks,
		StartTime:           fmt.Sprintf("%02d:%02d", hh, mm),
		DurationMin:         in.DurationMin,
		Capacity:            in.Capacity,
		Occurrences:         occ,
	})
}

// resolveEndDate 解析结束条件：end_date 与 repeat_weeks 二选一。
func (s *Service) resolveEndDate(in GenerateInput, startDate time.Time) (time.Time, error) {
	hasEnd := in.EndDate != ""
	hasRepeat := in.RepeatWeeks != nil
	if hasEnd == hasRepeat {
		return time.Time{}, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "结束日期与重复周数二选一", 400)
	}
	if hasRepeat {
		if *in.RepeatWeeks <= 0 {
			return time.Time{}, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "重复周数必须大于 0", 400)
		}
		// 覆盖 repeat_weeks 个整周（含起始周）：start + N*7 - 1 天。
		return startDate.AddDate(0, 0, *in.RepeatWeeks*7-1), nil
	}
	endDate, err := time.ParseInLocation(dateLayout, in.EndDate, cstZone)
	if err != nil {
		return time.Time{}, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "结束日期格式错误（YYYY-MM-DD）", 400)
	}
	if endDate.Before(startDate) {
		return time.Time{}, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "结束日期不能早于开始日期", 400)
	}
	return endDate, nil
}

// normalizeWeekdays 校验 + 去重 + 排序，0..6。
func normalizeWeekdays(in []int) ([]int, error) {
	if len(in) == 0 {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "请至少选择一个星期几", 400)
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(in))
	for _, w := range in {
		if w < 0 || w > 6 {
			return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "星期几取值必须为 0-6", 400)
		}
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	sort.Ints(out)
	return out, nil
}

func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return 0, 0, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "开始时间格式错误（HH:mm）", 400)
	}
	return t.Hour(), t.Minute(), nil
}

// buildOccurrences 在 [startDate, endDate] 内取 weekday 命中的日期，组装 starts/ends（UTC），
// 跳过已过去的时刻（starts <= now）。
func buildOccurrences(startDate, endDate time.Time, weekdays []int, hh, mm, durationMin int, now time.Time) []domainrec.Occurrence {
	wset := map[int]bool{}
	for _, w := range weekdays {
		wset[w] = true
	}
	var occ []domainrec.Occurrence
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if !wset[int(d.Weekday())] {
			continue
		}
		starts := time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, cstZone)
		if !starts.After(now) {
			continue // 过去的时刻不生成
		}
		ends := starts.Add(time.Duration(durationMin) * time.Minute)
		occ = append(occ, domainrec.Occurrence{
			StartsAt:  starts.UTC(),
			EndsAt:    ends.UTC(),
			DateLabel: starts.Format(dateLayout),
			TimeLabel: starts.Format(timeLayout),
		})
	}
	return occ
}

// List 列表（分页 + 过滤 + data_scope）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainrec.Schedule, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "session.view"); err != nil {
		return nil, 0, err
	}
	if in.LocationID > 0 {
		if err := s.guardLocationInScope(ctx, in.BrandID, in.ActorID, in.LocationID); err != nil {
			return nil, 0, err
		}
	}
	page := in.Page
	if page < 1 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	scopeIDs, err := s.scopeFilterIDs(ctx, in.BrandID, in.ActorID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, domainrec.ListFilter{
		BrandID:          in.BrandID,
		LocationID:       in.LocationID,
		Status:           in.Status,
		ScopeLocationIDs: scopeIDs,
	}, (page-1)*pageSize, pageSize)
}

// Get 详情（模板 + 已生成场次，data_scope 守卫）。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domainrec.Schedule, []*domainsession.Session, error) {
	if err := s.require(ctx, brandID, actorID, "session.view"); err != nil {
		return nil, nil, err
	}
	sch, sessions, err := s.repo.GetDetail(ctx, brandID, id)
	if err != nil {
		return nil, nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, sch.LocationID); err != nil {
		return nil, nil, err
	}
	return sch, sessions, nil
}

// Cancel 取消循环排课（非级联）。
func (s *Service) Cancel(ctx context.Context, brandID, actorID, id int64) (*domainrec.Schedule, error) {
	if err := s.require(ctx, brandID, actorID, "session.cancel"); err != nil {
		return nil, err
	}
	sch, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, sch.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Cancel(ctx, brandID, actorID, id)
}
