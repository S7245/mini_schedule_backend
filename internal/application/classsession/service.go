// Package classsession 课程场次应用服务（Batch 11 单场次排课）。
package classsession

import (
	"context"
	"time"

	domainsession "github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker 是 service 需要的最小 Checker 面（Require + Resolve，后者供 data_scope）。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 场次应用服务。
type Service struct {
	repo    domainsession.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope（兼容 bootstrap）。
func NewService(repo domainsession.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// scopeFilterIDs 把 actor 的 data_scope 转为 location id 过滤集（镜像 location.Service）。
// nil = all_brand 不限制；空切片 = 拒绝所有。
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

// guardLocationInScope 写/详情路径守卫：assigned_locations 时目标 location 必须在 scope 内，否则 404。
func (s *Service) guardLocationInScope(ctx context.Context, brandID, actorID, locationID int64) error {
	ids, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	if ids == nil {
		return nil // all_brand
	}
	for _, lid := range ids {
		if lid == locationID {
			return nil
		}
	}
	return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
}

// CreateInput 创建入参。
type CreateInput struct {
	BrandID             int64
	ActorID             int64
	CourseID            int64
	LocationID          int64
	LocationResourceID  *int64 // Batch 12a：可选绑定资源。
	InstructorProfileID int64
	StartsAt            time.Time
	EndsAt              time.Time
	Capacity            int
	WaitlistLimit       int
}

// ListInput 列表查询。
type ListInput struct {
	BrandID             int64
	ActorID             int64
	LocationID          int64
	CourseID            int64
	InstructorProfileID int64
	Status              string
	From                *time.Time
	To                  *time.Time
	Page                int
	PageSize            int
}

// List 列表（分页 + 过滤 + data_scope）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainsession.Session, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "session.view"); err != nil {
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
	return s.repo.List(ctx, domainsession.ListFilter{
		BrandID:             in.BrandID,
		LocationID:          in.LocationID,
		CourseID:            in.CourseID,
		InstructorProfileID: in.InstructorProfileID,
		Status:              in.Status,
		From:                in.From,
		To:                  in.To,
		ScopeLocationIDs:    scopeIDs,
	}, (page-1)*pageSize, pageSize)
}

// Get 详情（data_scope 守卫）。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domainsession.Session, error) {
	if err := s.require(ctx, brandID, actorID, "session.view"); err != nil {
		return nil, err
	}
	sess, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, sess.LocationID); err != nil {
		return nil, err
	}
	return sess, nil
}

// Create 单场次排课。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domainsession.Session, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "session.create"); err != nil {
		return nil, err
	}
	if in.CourseID <= 0 || in.LocationID <= 0 || in.InstructorProfileID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "课程 / 门店 / 教练不能为空", 400)
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "场次时间不合法", 400)
	}
	if !in.EndsAt.After(in.StartsAt) {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "结束时间必须晚于开始时间", 400)
	}
	if !in.StartsAt.After(time.Now().UTC()) {
		return nil, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "开始时间必须晚于当前时间", 400)
	}
	if in.WaitlistLimit < 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "候补上限不能为负", 400)
	}
	// data_scope：只能在 scope 内的门店排课。
	if err := s.guardLocationInScope(ctx, in.BrandID, in.ActorID, in.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, domainsession.CreateInput{
		BrandID:             in.BrandID,
		ActorID:             in.ActorID,
		CourseID:            in.CourseID,
		LocationID:          in.LocationID,
		LocationResourceID:  in.LocationResourceID,
		InstructorProfileID: in.InstructorProfileID,
		StartsAt:            in.StartsAt,
		EndsAt:              in.EndsAt,
		Capacity:            in.Capacity,
		WaitlistLimit:       in.WaitlistLimit,
	})
}

// Cancel 取消场次。
func (s *Service) Cancel(ctx context.Context, brandID, actorID, id int64, reason string) (*domainsession.Session, error) {
	if err := s.require(ctx, brandID, actorID, "session.cancel"); err != nil {
		return nil, err
	}
	sess, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, sess.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Cancel(ctx, brandID, actorID, id, reason)
}
