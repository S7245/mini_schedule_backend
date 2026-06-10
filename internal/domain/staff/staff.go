// Package staff 定义品牌员工（brand_user）聚合根。
//
// Batch 5 引入 "Staff" 概念，与 Batch 1 的 brand_users 表共用同一行；
// is_owner 列区分品牌负责人（不可删 / 不可降级）与其他员工。
package staff

import (
	"context"
	"time"
)

// Status staff 状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// IsValidStatus 校验状态字符串。
func IsValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}

// Staff 聚合根（brand_user 行的领域投影）。
type Staff struct {
	ID                  int64                `json:"id"`
	BrandID             int64                `json:"brand_id"`
	Phone               string               `json:"phone"`
	Name                string               `json:"name"`
	Status              Status               `json:"status"`
	IsOwner             bool                 `json:"is_owner"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	// 不用 omitempty：API 合约统一返数组（即便空），避免前端 .map() undefined 炸（owner 默认无 location）。
	RoleAssignments     []RoleAssignment     `json:"role_assignments"`
	LocationAssignments []LocationAssignment `json:"location_assignments"`
	HasInstructor       bool                 `json:"has_instructor"`
}

// RoleAssignment Staff 的角色任职关系（brand_user_role_assignments 行）。
type RoleAssignment struct {
	ID         int64   `json:"id"`
	RoleID     int64   `json:"role_id"`
	RoleCode   string  `json:"role_code"`
	RoleName   string  `json:"role_name"`
	ScopeType  string  `json:"scope_type"` // brand / location
	LocationID *int64  `json:"location_id,omitempty"`
	DataScope  string  `json:"data_scope"`
	Status     string  `json:"status"`
}

// LocationAssignment Staff 的 Location 任职关系。
type LocationAssignment struct {
	ID             int64  `json:"id"`
	LocationID     int64  `json:"location_id"`
	LocationName   string `json:"location_name,omitempty"`
	AssignmentType string `json:"assignment_type"` // member / manager / instructor / assistant
	IsPrimary      bool   `json:"is_primary"`
	Status         string `json:"status"`
}

// CreateInput POST /staff 入参。
type CreateInput struct {
	BrandID             int64
	ActorID             int64
	Phone               string
	Name                string
	InitialPassword     string
	RoleCodes           []string
	LocationAssignments []LocationAssignmentInput
}

// UpdateInput PATCH /staff/:id 入参（白名单，phone 不可改）。
type UpdateInput struct {
	Name *string
}

// LocationAssignmentInput PUT /staff/:id/location-assignments 单行。
type LocationAssignmentInput struct {
	LocationID     int64
	AssignmentType string
	IsPrimary      bool
}

// RoleAssignmentInput PUT /staff/:id/role-assignments 单行。
type RoleAssignmentInput struct {
	RoleCode   string
	LocationID *int64
	DataScope  string
}

// ListFilter 列表查询条件。
type ListFilter struct {
	BrandID       int64
	Status        string
	HasInstructor *bool
	Search        string
	// ScopeLocationIDs 非 nil 时按 data_scope 收紧：只返任职在这些 location 的 staff（Batch 6 T07）。
	// nil = 不限制（all_brand）；空切片 = 拒绝所有（DataScopeNone）。
	ScopeLocationIDs []int64
}

// Repository staff 仓储接口（DB 操作收敛在 infrastructure/persistence）。
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Staff, error)
	GetByID(ctx context.Context, brandID, id int64) (*Staff, error)
	GetWithAssignments(ctx context.Context, brandID, id int64) (*Staff, error)
	// InScopeLocations 判断 staff 是否任职在给定 location 集内（data_scope=assigned_locations 详情守卫用）。
	InScopeLocations(ctx context.Context, brandID, staffID int64, locationIDs []int64) (bool, error)
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Staff, int64, error)
	Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*Staff, error)
	UpdateStatus(ctx context.Context, brandID, actorID, id int64, status Status) (*Staff, error)
	SoftDelete(ctx context.Context, brandID, actorID, id int64) error

	// CountActiveOwners 用于 Owner 保护：if count==1 且目标命中 → 拒。
	CountActiveOwners(ctx context.Context, brandID int64) (int64, error)

	// ReplaceRoleAssignments 全量替换。事务内 DELETE 旧 + INSERT 新。
	ReplaceRoleAssignments(ctx context.Context, brandID, actorID, brandUserID int64, items []RoleAssignmentResolved) ([]RoleAssignment, error)

	// ReplaceLocationAssignments 全量替换。
	ReplaceLocationAssignments(ctx context.Context, brandID, actorID, brandUserID int64, items []LocationAssignmentInput) ([]LocationAssignment, error)
}

// RoleAssignmentResolved 是 application 层校验后的 role_id 解析结果，
// repository 不再回查 role_code。
type RoleAssignmentResolved struct {
	RoleID     int64
	ScopeType  string
	LocationID *int64
	DataScope  string
}
