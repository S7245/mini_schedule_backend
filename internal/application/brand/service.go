package brand

import (
	"context"

	"github.com/zkw/mini-schedule/backend/internal/domain/brand"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
)

// Service 品牌应用服务
type Service struct {
	repo   brand.Repository
	cfg    *config.Config
}

// NewService 创建品牌应用服务
func NewService(repo brand.Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// CreateBrand 创建品牌
func (s *Service) CreateBrand(ctx context.Context, input brand.CreateBrandInput) (*brand.Brand, error) {
	return s.repo.Create(ctx, input)
}

// GetBrand 获取品牌详情
func (s *Service) GetBrand(ctx context.Context, id int64) (*brand.Brand, error) {
	return s.repo.GetByID(ctx, id)
}

// ListBrands 获取品牌列表
func (s *Service) ListBrands(ctx context.Context, page, pageSize int) ([]*brand.Brand, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = s.cfg.Pagination.DefaultPageSize
	}
	if pageSize > s.cfg.Pagination.MaxPageSize {
		pageSize = s.cfg.Pagination.MaxPageSize
	}

	offset := (page - 1) * pageSize
	return s.repo.List(ctx, offset, pageSize)
}

// UpdateBrand 更新品牌
func (s *Service) UpdateBrand(ctx context.Context, id int64, input brand.UpdateBrandInput) (*brand.Brand, error) {
	return s.repo.Update(ctx, id, input)
}

// UpdateBrandStatus 更新品牌状态
func (s *Service) UpdateBrandStatus(ctx context.Context, id int64, status brand.Status) error {
	return s.repo.UpdateStatus(ctx, id, status)
}
