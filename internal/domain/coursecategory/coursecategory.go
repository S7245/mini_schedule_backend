// Package coursecategory 课程分类领域（course_categories 表）。
//
// 与 legacy internal/domain/course 分离，专供 brand 端 Batch 11 管理使用。
package coursecategory

import (
	"context"
	"time"
)

// Status 分类状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// IsValidStatus 校验状态字符串。
func IsValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}

// Category 课程分类实体。
type Category struct {
	ID                int64     `json:"id"`
	BrandID           int64     `json:"brand_id"`
	Name              string    `json:"name"`
	Color             string    `json:"color"`
	Icon              string    `json:"icon"`
	SortOrder         int       `json:"sort_order"`
	ShowInMiniProgram bool      `json:"show_in_mini_program"`
	Status            Status    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CreateInput 创建分类入参。
type CreateInput struct {
	BrandID           int64
	ActorID           int64
	Name              string
	Color             string
	Icon              string
	SortOrder         int
	ShowInMiniProgram bool
}

// UpdateInput 更新分类入参（白名单，nil = 不改）。
type UpdateInput struct {
	Name              *string
	Color             *string
	Icon              *string
	SortOrder         *int
	ShowInMiniProgram *bool
	Status            *string
}

// ListFilter 列表查询。
type ListFilter struct {
	BrandID int64
	Status  string // "active" / "inactive" / "" (= all)
}

// Repository 课程分类仓储接口。
type Repository interface {
	Create(ctx context.Context, in CreateInput) (*Category, error)
	GetByID(ctx context.Context, brandID, id int64) (*Category, error)
	List(ctx context.Context, filter ListFilter) ([]*Category, error)
	Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*Category, error)
	// CountActiveByIDs 统计给定 id 集合中属本 brand 且 active 的数量（用于课程模板校验 category_ids）。
	CountActiveByIDs(ctx context.Context, brandID int64, ids []int64) (int64, error)
}
