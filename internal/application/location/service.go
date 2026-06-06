package location

import (
	"context"
	"strings"

	domainlocation "github.com/zkw/mini-schedule/backend/internal/domain/location"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// Service Location 应用服务，编排 CRUD + quota（quota 校验下沉到 repository 事务内）。
type Service struct {
	repo domainlocation.Repository
}

// NewService 创建 Service。
func NewService(repo domainlocation.Repository) *Service {
	return &Service{repo: repo}
}

// CreateInput 创建入参。
type CreateInput struct {
	BrandID int64
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
	Status   string // "active" / "inactive" / "" / "all"
	Page     int
	PageSize int
}

// Create 创建门店；quota / subscription 校验在 repository 内单事务串行化。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domainlocation.Location, error) {
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
		Name:    in.Name,
		Address: in.Address,
		Phone:   in.Phone,
		Remark:  in.Remark,
	})
}

// Get 详情。
func (s *Service) Get(ctx context.Context, brandID, id int64) (*domainlocation.Location, error) {
	return s.repo.GetByID(ctx, brandID, id)
}

// List 列表（含分页 + 状态过滤）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainlocation.Location, int64, error) {
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

	return s.repo.List(ctx, domainlocation.ListLocationsFilter{
		BrandID: in.BrandID,
		Status:  status,
	}, (page-1)*pageSize, pageSize)
}

// Update 普通字段编辑。per 契约 Q5：本批不写 OperationLog（只创建 / 状态切换 / 删除 才写）。
func (s *Service) Update(ctx context.Context, brandID, id int64, in UpdateInput) (*domainlocation.Location, error) {
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
func (s *Service) UpdateStatus(ctx context.Context, brandID, id int64, status string) (*domainlocation.Location, error) {
	if !domainlocation.IsValidStatus(status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的门店状态", 400)
	}
	return s.repo.UpdateStatus(ctx, brandID, id, domainlocation.Status(status))
}

// Delete 软删。
func (s *Service) Delete(ctx context.Context, brandID, id int64) error {
	return s.repo.SoftDelete(ctx, brandID, id)
}
