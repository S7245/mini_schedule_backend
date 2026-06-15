package location

import (
	"context"
	"strings"

	domainlocation "github.com/zkw/mini-schedule/backend/internal/domain/location"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker is the minimal Checker surface location.Service needs.
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service Location 应用服务，编排 CRUD + quota（quota 校验下沉到 repository 事务内）。
type Service struct {
	repo    domainlocation.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过 RequirePermission（兼容 bootstrap 路径）。
func NewService(repo domainlocation.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// scopeFilterIDs 拿 actor 的 data_scope 转为 ListLocationsFilter.ScopeLocationIDs（Batch 6 T07）。
// nil = all_brand 不限制；空切片 = DataScopeNone 拒绝所有。
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

// guardLocationInScope 详情/写路径守卫：assigned_locations 时目标 location 必须在 scope 内，否则 404。
func (s *Service) guardLocationInScope(ctx context.Context, brandID, actorID, id int64) error {
	ids, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	if ids == nil {
		return nil // all_brand
	}
	for _, lid := range ids {
		if lid == id {
			return nil
		}
	}
	return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
}

// CreateInput 创建入参。
type CreateInput struct {
	BrandID int64
	ActorID int64 // brand_user_id（用于 OperationLog；0 表示未知 / 系统操作）
	Name    string
	Address string
	Phone   string
	Remark  string
}

// UpdateInput 更新入参（白名单）。
type UpdateInput struct {
	Name    *string
	Address *string
	Phone   *string
	Remark  *string
}

// ListInput 列表查询。
type ListInput struct {
	BrandID  int64
	ActorID  int64
	Status   string // "active" / "inactive" / "" / "all"
	Q        string // 门店名模糊搜索（Batch 10 T06）
	Page     int
	PageSize int
}

// Create 创建门店；quota / subscription 校验在 repository 内单事务串行化。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domainlocation.Location, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "location.create"); err != nil {
		return nil, err
	}
	if in.BrandID <= 0 {
		return nil, apperr.ErrBadRequest("品牌 ID 无效")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店名称不能为空", 400)
	}
	if len([]rune(in.Name)) > 100 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店名称过长", 400)
	}
	return s.repo.Create(ctx, domainlocation.CreateLocationInput{
		BrandID: in.BrandID,
		ActorID: in.ActorID,
		Name:    in.Name,
		Address: in.Address,
		Phone:   in.Phone,
		Remark:  in.Remark,
	})
}

// Get 详情。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domainlocation.Location, error) {
	if err := s.require(ctx, brandID, actorID, "location.view"); err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, brandID, id)
}

// List 列表（含分页 + 状态过滤）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainlocation.Location, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "location.view"); err != nil {
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

	status := in.Status
	if status == "all" {
		status = ""
	}

	// Batch 6 T07：按 actor 的 data_scope 收紧列表
	scopeIDs, err := s.scopeFilterIDs(ctx, in.BrandID, in.ActorID)
	if err != nil {
		return nil, 0, err
	}

	return s.repo.List(ctx, domainlocation.ListLocationsFilter{
		BrandID:          in.BrandID,
		Status:           status,
		Q:                strings.TrimSpace(in.Q),
		ScopeLocationIDs: scopeIDs,
	}, (page-1)*pageSize, pageSize)
}

// Update 普通字段编辑。per 契约 Q5：本批不写 OperationLog（只创建 / 状态切换 / 删除 才写）。
func (s *Service) Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*domainlocation.Location, error) {
	if err := s.require(ctx, brandID, actorID, "location.edit"); err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店名称不能为空", 400)
		}
		if len([]rune(v)) > 100 {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店名称过长", 400)
		}
		in.Name = &v
	}
	return s.repo.Update(ctx, brandID, id, domainlocation.UpdateLocationInput{
		Name:    in.Name,
		Address: in.Address,
		Phone:   in.Phone,
		Remark:  in.Remark,
	})
}

// UpdateStatus 状态切换（active / inactive）。
func (s *Service) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status string) (*domainlocation.Location, error) {
	if err := s.require(ctx, brandID, actorID, "location.edit"); err != nil {
		return nil, err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	if !domainlocation.IsValidStatus(status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的门店状态", 400)
	}
	return s.repo.UpdateStatus(ctx, brandID, actorID, id, domainlocation.Status(status))
}

// Delete 软删。
func (s *Service) Delete(ctx context.Context, brandID, actorID, id int64) error {
	if err := s.require(ctx, brandID, actorID, "location.delete"); err != nil {
		return err
	}
	if err := s.guardLocationInScope(ctx, brandID, actorID, id); err != nil {
		return err
	}
	// Batch 9：软删不触发 FK，先查 active 引用（员工任职 + 门店级角色任职），有则拒删。
	n, err := s.repo.CountActiveReferences(ctx, brandID, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return apperr.NewAppError(apperr.ErrLocationInUse, "该门店仍有员工任职或角色绑定，请先移除后再删除", 409)
	}
	return s.repo.SoftDelete(ctx, brandID, actorID, id)
}
