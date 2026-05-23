package user

import "context"

// AppUserRepository C 端用户仓储接口
type AppUserRepository interface {
	Create(ctx context.Context, input CreateAppUserInput) (*AppUser, error)
	GetByID(ctx context.Context, id int64) (*AppUser, error)
	GetByBrandIDAndOpenID(ctx context.Context, brandID int64, openID string) (*AppUser, error)
	ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*AppUser, int64, error)
	Update(ctx context.Context, id int64, nickname, avatarURL string) (*AppUser, error)
	UpdateVIPLevel(ctx context.Context, id int64, level VIPLevel) error
}

// BrandUserRepository 品牌管理员仓储接口
type BrandUserRepository interface {
	Create(ctx context.Context, input CreateBrandUserInput) (*BrandUser, error)
	GetByID(ctx context.Context, id int64) (*BrandUser, error)
	GetByPhone(ctx context.Context, phone string) (*BrandUser, error)
	ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*BrandUser, int64, error)
	UpdateStatus(ctx context.Context, id int64, status Status) error
}

// AdminUserRepository 平台管理员仓储接口
type AdminUserRepository interface {
	Create(ctx context.Context, input CreateAdminUserInput) (*AdminUser, error)
	GetByID(ctx context.Context, id int64) (*AdminUser, error)
	GetByUsername(ctx context.Context, username string) (*AdminUser, error)
	List(ctx context.Context, offset, limit int) ([]*AdminUser, int64, error)
}
