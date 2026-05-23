package training

import "context"

// Repository 训练记录仓储接口
type Repository interface {
	Create(ctx context.Context, input CreateRecordInput) (*Record, error)
	GetByID(ctx context.Context, id int64) (*Record, error)
	ListByUserID(ctx context.Context, userID int64, offset, limit int) ([]*Record, int64, error)
	ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*Record, int64, error)
}
