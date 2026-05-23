package training

import (
	"context"

	"github.com/zkw/mini-schedule/backend/internal/domain/training"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
)

// Service 训练记录应用服务
type Service struct {
	repo training.Repository
	cfg  *config.Config
}

// NewService 创建训练记录应用服务
func NewService(repo training.Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) CreateRecord(ctx context.Context, input training.CreateRecordInput) (*training.Record, error) {
	return s.repo.Create(ctx, input)
}

func (s *Service) GetRecord(ctx context.Context, id int64) (*training.Record, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListByUser(ctx context.Context, userID int64, page, pageSize int) ([]*training.Record, int64, error) {
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
	return s.repo.ListByUserID(ctx, userID, offset, pageSize)
}

func (s *Service) ListByBrand(ctx context.Context, brandID int64, page, pageSize int) ([]*training.Record, int64, error) {
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
	return s.repo.ListByBrandID(ctx, brandID, offset, pageSize)
}
