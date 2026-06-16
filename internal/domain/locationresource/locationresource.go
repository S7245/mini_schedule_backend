// Package locationresource 门店资源领域（location_resources 表，Batch 12a）。
//
// LocationResource 是 Location 下可被排课占用的资源（教室/场地/线上/设备）。场次可选绑定；
// 同一资源同一时段不能被两个有效场次占用，由 DB EXCLUDE 约束 class_sessions_resource_no_overlap
// 兜底（SQLSTATE 23P01）。资源停用后不能新排课；资源容量可作场次容量默认值。
package locationresource

import (
	"context"
	"time"
)

// Status 资源状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// IsValidStatus 判断输入字符串是否合法状态值。
func IsValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}

// Type 资源类型（与 DB CHECK location_resources_type_valid 对齐）。
type Type string

const (
	TypeClassroom Type = "classroom"
	TypeVenue     Type = "venue"
	TypeOnline    Type = "online"
	TypeEquipment Type = "equipment"
	TypeOther     Type = "other"
)

// IsValidType 判断输入字符串是否合法资源类型。
func IsValidType(s string) bool {
	switch Type(s) {
	case TypeClassroom, TypeVenue, TypeOnline, TypeEquipment, TypeOther:
		return true
	}
	return false
}

// Resource 资源实体（含列表用反范式 location_name）。
type Resource struct {
	ID           int64     `json:"id"`
	BrandID      int64     `json:"brand_id"`
	LocationID   int64     `json:"location_id"`
	Name         string    `json:"name"`
	Type         Type      `json:"type"`
	Capacity     int       `json:"capacity"`
	Status       Status    `json:"status"`
	Remark       string    `json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LocationName string    `json:"location_name"` // 反范式（JOIN 取）。
}

// CreateInput 创建入参。Capacity <= 0 时由 repo 取 DB 默认 1。
type CreateInput struct {
	BrandID    int64
	ActorID    int64
	LocationID int64
	Name       string
	Type       string
	Capacity   int
	Remark     string
}

// UpdateInput 更新入参（白名单）。LocationID 不可改（资源不跨门店迁移）。
type UpdateInput struct {
	Name     *string
	Type     *string
	Capacity *int
	Status   *string
	Remark   *string
}

// ListFilter 列表查询。零值不过滤；ScopeLocationIDs 非 nil 时按 data_scope 收紧。
type ListFilter struct {
	BrandID          int64
	LocationID       int64
	Status           string
	ScopeLocationIDs []int64
}

// Repository 资源仓储接口。
type Repository interface {
	// Create 校验 location 属本 brand + active，落 active 资源；同门店重名 → RESOURCE_NAME_DUPLICATED。
	Create(ctx context.Context, in CreateInput) (*Resource, error)
	GetByID(ctx context.Context, brandID, id int64) (*Resource, error)
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Resource, int64, error)
	Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*Resource, error)
	// Delete 软删；被 scheduled/in_progress 场次或 active 循环排课引用 → RESOURCE_IN_USE。
	Delete(ctx context.Context, brandID, actorID, id int64) error
}
