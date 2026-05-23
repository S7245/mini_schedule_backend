package brand

import "context"

// Repository 品牌仓储接口
type Repository interface {
	Create(ctx context.Context, input CreateBrandInput) (*Brand, error)
	GetByID(ctx context.Context, id int64) (*Brand, error)
	List(ctx context.Context, offset, limit int) ([]*Brand, int64, error)
	Update(ctx context.Context, id int64, input UpdateBrandInput) (*Brand, error)
	UpdateStatus(ctx context.Context, id int64, status Status) error
}
