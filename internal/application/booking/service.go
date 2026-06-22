// Package booking 预约下单应用服务（Batch 13c）。
//
// 权限：booking.view（查看）/ booking.create_assisted（代预约）/ booking.cancel（代取消）；
// 预约策略读写复用 schedule.view / schedule.manage（决策 1：仅 brand-default 策略读改，零新权限码）。
// data_scope：按 class_session.location_id ∈ assigned_locations 守卫（镜像 classsession.Service）。
package booking

import (
	"context"

	domainbooking "github.com/zkw/mini-schedule/backend/internal/domain/booking"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker service 需要的最小 Checker 面（Require + Resolve）。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 预约应用服务。
type Service struct {
	repo    domainbooking.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope（兼容 bootstrap）。
func NewService(repo domainbooking.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// scopeFilterIDs 把 actor 的 data_scope 转为 location id 过滤集。nil = all_brand 不限；空切片 = 拒绝所有。
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

// guardLocationInScope 写/详情守卫：assigned_locations 时目标 location 须在 scope 内，否则 404（不泄漏存在性）。
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
	return apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
}

// CreateInput 代预约入参。
type CreateInput struct {
	BrandID               int64
	ActorID               int64
	ClassSessionID        int64
	BrandLearnerProfileID int64
	EntitlementMode       string
	LearnerEntitlementID  *int64
	NoEntitlementReason   string
}

// Create 代预约（booking.create_assisted + data_scope 在 repo 内按 ScopeLocationIDs 守卫）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domainbooking.Booking, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "booking.create_assisted"); err != nil {
		return nil, err
	}
	if in.ClassSessionID <= 0 || in.BrandLearnerProfileID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "场次和学员不能为空", 400)
	}
	if !domainbooking.IsValidEntitlementMode(in.EntitlementMode) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的权益模式", 400)
	}
	scopeIDs, err := s.scopeFilterIDs(ctx, in.BrandID, in.ActorID)
	if err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, domainbooking.CreateInput{
		BrandID:               in.BrandID,
		ActorID:               in.ActorID,
		ClassSessionID:        in.ClassSessionID,
		BrandLearnerProfileID: in.BrandLearnerProfileID,
		EntitlementMode:       domainbooking.EntitlementMode(in.EntitlementMode),
		LearnerEntitlementID:  in.LearnerEntitlementID,
		NoEntitlementReason:   in.NoEntitlementReason,
		ScopeLocationIDs:      scopeIDs,
	})
}

// Cancel 代取消（booking.cancel + data_scope 守卫场次门店）。
func (s *Service) Cancel(ctx context.Context, brandID, actorID, id int64, reason string) (*domainbooking.Booking, error) {
	if err := s.require(ctx, brandID, actorID, "booking.cancel"); err != nil {
		return nil, err
	}
	bk, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, bk.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Cancel(ctx, brandID, actorID, id, reason)
}

// ListInput 列表查询。
type ListInput struct {
	BrandID                int64
	ActorID                int64
	ClassSessionID         int64
	LocationID             int64
	BrandLearnerProfileID  int64
	Status                 string
	RequiresEntitlementFix *bool
	Page                   int
	PageSize               int
}

// List 预约列表（booking.view + 分页 + data_scope）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainbooking.Booking, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "booking.view"); err != nil {
		return nil, 0, err
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
	return s.repo.List(ctx, domainbooking.ListFilter{
		BrandID:                in.BrandID,
		ClassSessionID:         in.ClassSessionID,
		LocationID:             in.LocationID,
		BrandLearnerProfileID:  in.BrandLearnerProfileID,
		Status:                 in.Status,
		RequiresEntitlementFix: in.RequiresEntitlementFix,
		ScopeLocationIDs:       scopeIDs,
	}, (page-1)*pageSize, pageSize)
}

// Get 预约详情（booking.view + data_scope 守卫）。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domainbooking.Booking, error) {
	if err := s.require(ctx, brandID, actorID, "booking.view"); err != nil {
		return nil, err
	}
	bk, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, bk.LocationID); err != nil {
		return nil, err
	}
	return bk, nil
}

// UsableEntitlements 某学员对某场次的可用权益预览（booking.view + data_scope 守卫场次）。
func (s *Service) UsableEntitlements(ctx context.Context, brandID, actorID, sessionID, learnerID int64) ([]*domainbooking.UsableEntitlement, error) {
	if err := s.require(ctx, brandID, actorID, "booking.view"); err != nil {
		return nil, err
	}
	scopeIDs, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return nil, err
	}
	return s.repo.UsableEntitlements(ctx, brandID, sessionID, learnerID, scopeIDs)
}

// GetPolicy 读 brand-default 预约策略（schedule.view）。
func (s *Service) GetPolicy(ctx context.Context, brandID, actorID int64) (*domainbooking.Policy, error) {
	if err := s.require(ctx, brandID, actorID, "schedule.view"); err != nil {
		return nil, err
	}
	return s.repo.GetDefaultPolicy(ctx, brandID)
}

// UpsertPolicy 更新 brand-default 预约策略（schedule.manage）。
func (s *Service) UpsertPolicy(ctx context.Context, brandID, actorID int64, p domainbooking.Policy) (*domainbooking.Policy, error) {
	if err := s.require(ctx, brandID, actorID, "schedule.manage"); err != nil {
		return nil, err
	}
	if p.BookAheadMinMinutes < 0 || p.CancelDeadlineMinutes < 0 || p.WaitlistLimit < 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "时间/上限不能为负", 400)
	}
	if p.BookAheadMaxMinutes != nil && *p.BookAheadMaxMinutes < 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "最多提前时间不能为负", 400)
	}
	for _, lim := range []*int{p.DailyBookingLimit, p.WeeklyBookingLimit, p.ConcurrentBookingLimit} {
		if lim != nil && *lim <= 0 {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "频次上限须为正（不限请留空）", 400)
		}
	}
	return s.repo.UpsertDefaultPolicy(ctx, brandID, actorID, p)
}
