package location

import (
	"context"
	"time"
)

// Status 门店状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// IsValidStatus 判断输入字符串是否合法状态值。
func IsValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}

// Location 门店实体（聚合根）。
type Location struct {
	ID        int64     `json:"id"`
	BrandID   int64     `json:"brand_id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Phone     string    `json:"phone"`
	Status    Status    `json:"status"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateLocationInput POST 入参。
type CreateLocationInput struct {
	BrandID int64
	Name    string
	Address string
	Phone   string
	Remark  string
}

// UpdateLocationInput PATCH 入参（白名单可修改字段）。
type UpdateLocationInput struct {
	Name    *string
	Address *string
	Phone   *string
	Remark  *string
}

// ListLocationsFilter 列表查询条件。
type ListLocationsFilter struct {
	BrandID int64
	Status  string // "active" / "inactive" / "" (= all)
}

// Repository 门店仓储接口。
type Repository interface {
	// Create 在同一事务内做 quota 校验（SELECT FOR UPDATE subscription + COUNT active locations + INSERT）。
	Create(ctx context.Context, input CreateLocationInput) (*Location, error)

	GetByID(ctx context.Context, brandID, id int64) (*Location, error)
	List(ctx context.Context, filter ListLocationsFilter, offset, limit int) ([]*Location, int64, error)
	Update(ctx context.Context, brandID, id int64, input UpdateLocationInput) (*Location, error)
	UpdateStatus(ctx context.Context, brandID, id int64, status Status) (*Location, error)
	SoftDelete(ctx context.Context, brandID, id int64) error
}
