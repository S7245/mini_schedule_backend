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
	ActorID int64 // 操作的 brand_user_id，用于 OperationLog 留痕
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
	// Q 门店名模糊搜索（大小写不敏感，ILIKE）。空字符串 = 不过滤（Batch 10 T06）。
	Q string
	// ScopeLocationIDs 非 nil 时按 data_scope 收紧：只返这些 id 的 location（Batch 6 T07）。
	// nil = 不限制；空切片 = 拒绝所有。
	ScopeLocationIDs []int64
}

// Repository 门店仓储接口。
type Repository interface {
	// Create 在同一事务内做 quota 校验（SELECT FOR UPDATE subscription + COUNT active locations + INSERT）。
	Create(ctx context.Context, input CreateLocationInput) (*Location, error)

	GetByID(ctx context.Context, brandID, id int64) (*Location, error)
	List(ctx context.Context, filter ListLocationsFilter, offset, limit int) ([]*Location, int64, error)
	Update(ctx context.Context, brandID, id int64, input UpdateLocationInput) (*Location, error)
	UpdateStatus(ctx context.Context, brandID, actorID, id int64, status Status) (*Location, error)
	SoftDelete(ctx context.Context, brandID, actorID, id int64) error
	// CountActiveReferences 统计阻止删除的 active 引用（员工任职 + 门店级角色任职），带 brand_id 隔离。
	// Batch 9：删除门店前校验，>0 → LOCATION_IN_USE。
	CountActiveReferences(ctx context.Context, brandID, locationID int64) (int64, error)
}
