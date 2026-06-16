// Package locationresource 门店资源应用服务（Batch 12a）。
package locationresource

import (
	"context"
	"strings"

	domainres "github.com/zkw/mini-schedule/backend/internal/domain/locationresource"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker 是 service 需要的最小 Checker 面（Require + Resolve，后者供 data_scope）。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 资源应用服务。
type Service struct {
	repo    domainres.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope（兼容 bootstrap）。
func NewService(repo domainres.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// scopeFilterIDs 把 actor 的 data_scope 转为 location id 过滤集（镜像 classsession.Service）。
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
	return apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
}

// CreateInput 创建入参。
type CreateInput struct {
	BrandID    int64
	ActorID    int64
	LocationID int64
	Name       string
	Type       string
	Capacity   int
	Remark     string
}

// UpdateInput 更新入参（白名单）。
type UpdateInput struct {
	Name     *string
	Type     *string
	Capacity *int
	Status   *string
	Remark   *string
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

// List 列表（分页 + 过滤 + data_scope）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainres.Resource, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "location_resource.view"); err != nil {
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
	return s.repo.List(ctx, domainres.ListFilter{
		BrandID:          in.BrandID,
		LocationID:       in.LocationID,
		Status:           in.Status,
		ScopeLocationIDs: scopeIDs,
	}, (page-1)*pageSize, pageSize)
}

// Get 详情（data_scope 守卫）。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domainres.Resource, error) {
	if err := s.require(ctx, brandID, actorID, "location_resource.view"); err != nil {
		return nil, err
	}
	res, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, res.LocationID); err != nil {
		return nil, err
	}
	return res, nil
}

// Create 创建资源。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domainres.Resource, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "location_resource.create"); err != nil {
		return nil, err
	}
	if in.LocationID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店不能为空", 400)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "资源名称不能为空", 400)
	}
	if len([]rune(name)) > 100 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "资源名称过长", 400)
	}
	if !domainres.IsValidType(in.Type) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的资源类型", 400)
	}
	if err := s.guardLocationInScope(ctx, in.BrandID, in.ActorID, in.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, domainres.CreateInput{
		BrandID:    in.BrandID,
		ActorID:    in.ActorID,
		LocationID: in.LocationID,
		Name:       name,
		Type:       in.Type,
		Capacity:   in.Capacity,
		Remark:     in.Remark,
	})
}

// Update 编辑资源（含状态切换）。
func (s *Service) Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*domainres.Resource, error) {
	if err := s.require(ctx, brandID, actorID, "location_resource.edit"); err != nil {
		return nil, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "资源名称不能为空", 400)
		}
		if len([]rune(v)) > 100 {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "资源名称过长", 400)
		}
		in.Name = &v
	}
	if in.Type != nil && !domainres.IsValidType(*in.Type) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的资源类型", 400)
	}
	if in.Status != nil && !domainres.IsValidStatus(*in.Status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的资源状态", 400)
	}
	if in.Capacity != nil && *in.Capacity <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "容量必须大于 0", 400)
	}
	// data_scope：仅能改 scope 内门店的资源。
	res, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, res.LocationID); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, brandID, actorID, id, domainres.UpdateInput{
		Name:     in.Name,
		Type:     in.Type,
		Capacity: in.Capacity,
		Status:   in.Status,
		Remark:   in.Remark,
	})
}

// Delete 软删资源（引用保护 + data_scope 守卫）。
func (s *Service) Delete(ctx context.Context, brandID, actorID, id int64) error {
	if err := s.require(ctx, brandID, actorID, "location_resource.delete"); err != nil {
		return err
	}
	res, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, res.LocationID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, brandID, actorID, id)
}
