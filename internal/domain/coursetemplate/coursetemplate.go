// Package coursetemplate 课程模板领域（courses 表，Batch 11 brand 视角）。
//
// courses 表既被 legacy api-app 只读消费（internal/domain/course，含健身 difficulty/type），
// 也是 PDS 的 CourseTemplate。本包专供 brand 端管理：分类绑定 + 可用门店 + 发布状态机，
// 不写 difficulty/type（migration 000007 已 DROP NOT NULL，新模板这两列为 NULL）。
package coursetemplate

import (
	"context"
	"time"
)

// Status 课程模板状态机：draft → published → archived。
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

// IsValidStatus 校验状态字符串。
func IsValidStatus(s string) bool {
	switch Status(s) {
	case StatusDraft, StatusPublished, StatusArchived:
		return true
	}
	return false
}

// CategoryRef 课程所属分类的精简引用（列表 + 详情用）。
type CategoryRef struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Template 课程模板实体。
type Template struct {
	ID                int64        `json:"id"`
	BrandID           int64        `json:"brand_id"`
	Title             string       `json:"title"`
	Description       string       `json:"description"`
	CoverURL          string       `json:"cover_url"`
	LevelLabel        string       `json:"level_label"`
	DurationMin       int          `json:"duration_min"`
	DefaultCapacity   int          `json:"default_capacity"`
	ShowInMiniProgram bool         `json:"show_in_mini_program"`
	Status            Status       `json:"status"`
	PublishedAt       *time.Time   `json:"published_at"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	// Categories 列表 + 详情都返回（精简）。永不为 nil（前端 .map）。
	Categories []CategoryRef `json:"categories"`
	// AvailableLocationCount 列表用。
	AvailableLocationCount int `json:"available_location_count"`
	// AvailableLocationIDs / CategoryIDs 仅详情返回（编辑回填）。永不为 nil。
	AvailableLocationIDs []int64 `json:"available_location_ids,omitempty"`
	CategoryIDs          []int64 `json:"category_ids,omitempty"`
}

// CreateInput 创建入参。LocationIDs 为空 = 默认全选当前 active 门店。
type CreateInput struct {
	BrandID           int64
	ActorID           int64
	Title             string
	Description       string
	CoverURL          string
	LevelLabel        string
	DurationMin       int
	DefaultCapacity   int
	ShowInMiniProgram bool
	CategoryIDs       []int64
	LocationIDs       []int64
}

// UpdateInput 更新入参（白名单，nil = 不改）。CategoryIDs / LocationIDs 非 nil 时全量替换。
type UpdateInput struct {
	Title             *string
	Description       *string
	CoverURL          *string
	LevelLabel        *string
	DurationMin       *int
	DefaultCapacity   *int
	ShowInMiniProgram *bool
	CategoryIDs       *[]int64
	LocationIDs       *[]int64
}

// ListFilter 列表查询。
type ListFilter struct {
	BrandID    int64
	Status     string // draft/published/archived/"" (= all)
	Q          string // title ILIKE
	CategoryID int64  // 0 = 不过滤
}

// Repository 课程模板仓储接口。
type Repository interface {
	// Create 单事务：校验 category_ids/location_ids 属本 brand active → INSERT course
	// + category_assignments + location_availability + OperationLog。
	Create(ctx context.Context, in CreateInput) (*Template, error)
	// GetByID 含 Categories + CategoryIDs + AvailableLocationIDs。
	GetByID(ctx context.Context, brandID, id int64) (*Template, error)
	// List 含 Categories + AvailableLocationCount（分页）。
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Template, int64, error)
	// Update 单事务：白名单字段 + 全量替换 assignments/availability。
	Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*Template, error)
	// UpdateStatus 状态切换；→published 时首次置 published_at。
	UpdateStatus(ctx context.Context, brandID, actorID, id int64, status Status) (*Template, error)
	// SoftDelete 软删（deleted_at）。
	SoftDelete(ctx context.Context, brandID, actorID, id int64) error
	// CountScheduledSessions 统计阻止删除的 scheduled/in_progress 场次引用（COURSE_IN_USE）。
	CountScheduledSessions(ctx context.Context, brandID, courseID int64) (int64, error)
}
