package course

import (
	"context"

	"github.com/zkw/mini-schedule/backend/internal/domain/course"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
)

// Service 课程应用服务
type Service struct {
	repo course.Repository
	cfg  *config.Config
}

// NewService 创建课程应用服务
func NewService(repo course.Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) CreateCourse(ctx context.Context, input course.CreateCourseInput) (*course.Course, error) {
	return s.repo.Create(ctx, input)
}

func (s *Service) GetCourse(ctx context.Context, id int64) (*course.Course, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListCoursesByBrand(ctx context.Context, brandID int64, page, pageSize int) ([]*course.Course, int64, error) {
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

func (s *Service) ListPublishedCourses(ctx context.Context, brandID int64, page, pageSize int) ([]*course.Course, int64, error) {
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
	return s.repo.ListPublished(ctx, brandID, offset, pageSize)
}

func (s *Service) UpdateCourse(ctx context.Context, id int64, input course.UpdateCourseInput) (*course.Course, error) {
	return s.repo.Update(ctx, id, input)
}

func (s *Service) UpdateCourseStatus(ctx context.Context, id int64, status course.Status) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) DeleteCourse(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
