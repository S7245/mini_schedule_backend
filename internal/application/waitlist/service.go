// Package waitlist 候补应用服务（Batch 13d）。
//
// 权限复用 booking.*：booking.view（查看名单）/ booking.create_assisted（加入/转正/跳过）/
// booking.cancel（取消候补）。data_scope 按 class_session.location_id ∈ assigned_locations（越权 404）。
package waitlist

import (
	"context"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainwaitlist "github.com/zkw/mini-schedule/backend/internal/domain/waitlist"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker service 需要的最小 Checker 面。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 候补应用服务。
type Service struct {
	repo    domainwaitlist.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope（兼容 bootstrap）。
func NewService(repo domainwaitlist.Repository, checker PermissionChecker) *Service {
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

// guardLocationInScope 写守卫：assigned_locations 时目标 location 须在 scope 内，否则 404（不泄漏存在性）。
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
	return apperr.NewAppError(apperr.ErrWaitlistEntryNotFound, "候补不存在", 404)
}

// Join 加入候补（booking.create_assisted + data_scope 在 repo 内按 scope 守卫场次）。
func (s *Service) Join(ctx context.Context, brandID, actorID, sessionID, learnerID int64) (*domainwaitlist.Entry, error) {
	if err := s.require(ctx, brandID, actorID, "booking.create_assisted"); err != nil {
		return nil, err
	}
	if sessionID <= 0 || learnerID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "场次和学员不能为空", 400)
	}
	scopeIDs, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return nil, err
	}
	return s.repo.Join(ctx, domainwaitlist.JoinInput{
		BrandID: brandID, ActorID: actorID, ClassSessionID: sessionID,
		BrandLearnerProfileID: learnerID, ScopeLocationIDs: scopeIDs,
	})
}

// List 查看某场次候补名单（booking.view + data_scope）。
func (s *Service) List(ctx context.Context, brandID, actorID, sessionID int64) ([]*domainwaitlist.Entry, error) {
	if err := s.require(ctx, brandID, actorID, "booking.view"); err != nil {
		return nil, err
	}
	scopeIDs, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListBySession(ctx, brandID, sessionID, scopeIDs)
}

// PromoteParams 转正入参。
type PromoteParams struct {
	BrandID, ActorID, EntryID int64
	EntitlementMode           string
	LearnerEntitlementID      *int64
	NoEntitlementReason       string
}

// Promote 手动转正（booking.create_assisted + data_scope 守卫场次门店）。
func (s *Service) Promote(ctx context.Context, p PromoteParams) (*domainwaitlist.Entry, error) {
	if err := s.require(ctx, p.BrandID, p.ActorID, "booking.create_assisted"); err != nil {
		return nil, err
	}
	// 权益模式（auto/manual/none）合法性由下游 placeBooking 校验。
	entry, err := s.repo.GetByID(ctx, p.BrandID, p.EntryID)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, p.BrandID, p.ActorID, entry.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Promote(ctx, domainwaitlist.PromoteInput{
		BrandID: p.BrandID, ActorID: p.ActorID, EntryID: p.EntryID,
		EntitlementMode: p.EntitlementMode, LearnerEntitlementID: p.LearnerEntitlementID, NoEntitlementReason: p.NoEntitlementReason,
	})
}

// Skip 跳过候补（booking.create_assisted + data_scope）。
func (s *Service) Skip(ctx context.Context, brandID, actorID, id int64, reason string) (*domainwaitlist.Entry, error) {
	if err := s.require(ctx, brandID, actorID, "booking.create_assisted"); err != nil {
		return nil, err
	}
	entry, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, entry.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Skip(ctx, brandID, actorID, id, reason)
}

// Cancel 取消候补（booking.cancel + data_scope）。
func (s *Service) Cancel(ctx context.Context, brandID, actorID, id int64) (*domainwaitlist.Entry, error) {
	if err := s.require(ctx, brandID, actorID, "booking.cancel"); err != nil {
		return nil, err
	}
	entry, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, entry.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Cancel(ctx, brandID, actorID, id)
}
