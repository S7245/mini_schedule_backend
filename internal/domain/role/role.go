// Package role 暴露品牌角色 / 权限的领域类型与查询接口。
//
// 本批（Batch 5）read-only：
//   - 注册流程 / backfill 时从 role_templates 复制到 brand_roles；
//   - staff service 校验 role_code 合法性 + 决定 INSERT 的 brand_user_role_assignments.role_id；
//   - Batch 6 才会引入"品牌自定义角色 CRUD"。
package role

import (
	"context"
)

const (
	ScopeBrand    = "brand"
	ScopeLocation = "location"

	DataScopeRoleDefault       = "role_default"
	DataScopeAllBrand          = "all_brand"
	DataScopeAssignedLocations = "assigned_locations"
	DataScopeOwnSessions       = "own_sessions"
	DataScopeOwnRecords        = "own_records"
)

// IsValidDataScope 校验 data_scope 枚举（与 migration check constraint 一致）。
func IsValidDataScope(s string) bool {
	switch s {
	case DataScopeRoleDefault, DataScopeAllBrand, DataScopeAssignedLocations,
		DataScopeOwnSessions, DataScopeOwnRecords:
		return true
	}
	return false
}

// BrandRole brand_roles 表的领域投影。
type BrandRole struct {
	ID          int64        `json:"id"`
	BrandID     int64        `json:"brand_id"`
	TemplateID  *int64       `json:"template_id,omitempty"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	ScopeType   string       `json:"scope_type"`
	IsSystem    bool         `json:"is_system"`
	Status      string       `json:"status"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions,omitempty"`
}

// Permission permissions 表的领域投影。
type Permission struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Domain      string `json:"domain"`
	Action      string `json:"action"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

// RoleTemplate role_templates 表的领域投影（用于 backfill 时复制）。
type RoleTemplate struct {
	ID          int64        `json:"id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	ScopeType   string       `json:"scope_type"`
	Description string       `json:"description,omitempty"`
	Status      string       `json:"status"`
	Permissions []Permission `json:"permissions,omitempty"`
}

// CreateBrandRoleInput 新建自定义角色入参（Batch 7）。
// PermissionCodes 存原始勾选（不展开）；repo 负责把 code 解析成 permission_id。
type CreateBrandRoleInput struct {
	BrandID         int64
	ActorID         int64
	Name            string
	ScopeType       string
	Description     string
	PermissionCodes []string
}

// UpdateBrandRoleInput 编辑自定义角色入参（Batch 7）。
// scope_type 不可改（A3），故不在入参里。PermissionCodes 全量替换。
type UpdateBrandRoleInput struct {
	BrandID         int64
	ActorID         int64
	RoleID          int64
	Name            string
	Description     string
	PermissionCodes []string
}

// Repository 角色 / 权限查询接口。
type Repository interface {
	ListBrandRoles(ctx context.Context, brandID int64) ([]*BrandRole, error)
	GetBrandRoleByCode(ctx context.Context, brandID int64, code string) (*BrandRole, error)
	ListRoleTemplatesWithPermissions(ctx context.Context) ([]*RoleTemplate, error)

	// Batch 7 — 自定义角色 CRUD。
	ListPermissions(ctx context.Context) ([]Permission, error)
	CreateBrandRole(ctx context.Context, in CreateBrandRoleInput) (*BrandRole, error)
	UpdateBrandRole(ctx context.Context, in UpdateBrandRoleInput) (*BrandRole, error)
	UpdateBrandRoleStatus(ctx context.Context, brandID, actorID, roleID int64, status string) (*BrandRole, error)
	DeleteBrandRole(ctx context.Context, brandID, actorID, roleID int64) error
	CountAssignmentsByRole(ctx context.Context, roleID int64) (int64, error)
	ListBrandUserIDsByRole(ctx context.Context, roleID int64) ([]int64, error)
	// ListRolePermissionCodes 返回该角色当前已落库的原始权限 code 集合（B1 增量提权校验用）。
	ListRolePermissionCodes(ctx context.Context, roleID int64) ([]string, error)
}
