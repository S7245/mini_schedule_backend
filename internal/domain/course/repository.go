package course

import "context"

// Repository 课程仓储接口
type Repository interface {
	Create(ctx context.Context, input CreateCourseInput) (*Course, error)
	GetByID(ctx context.Context, id int64) (*Course, error)
	ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*Course, int64, error)
	ListPublished(ctx context.Context, brandID int64, offset, limit int) ([]*Course, int64, error)
	Update(ctx context.Context, id int64, input UpdateCourseInput) (*Course, error)
	UpdateStatus(ctx context.Context, id int64, status Status) error
	Delete(ctx context.Context, id int64) error
}
