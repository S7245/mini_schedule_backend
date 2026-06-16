// Package classsession 课程场次领域（class_sessions 表，Batch 11 单场次排课）。
//
// 本批只做单场次创建 / 列表 / 详情 / 取消；循环排课（recurring_schedules）+ 资源占用
// 推迟到 Batch 12。教练时间冲突由 DB EXCLUDE 约束兜底（SQLSTATE 23P01）。
package classsession

import (
	"context"
	"time"
)

// Status 场次状态机。本批创建直接落 scheduled（跳过 draft，使 EXCLUDE 约束 + onboarding 计数即时生效）。
type Status string

const (
	StatusDraft      Status = "draft"
	StatusScheduled  Status = "scheduled"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

// Session 场次实体（含列表/详情用的反范式名称字段）。
type Session struct {
	ID                  int64      `json:"id"`
	BrandID             int64      `json:"brand_id"`
	LocationID          int64      `json:"location_id"`
	LocationResourceID  *int64     `json:"location_resource_id"`
	CourseID            int64      `json:"course_id"`
	InstructorProfileID int64      `json:"instructor_profile_id"`
	StartsAt            time.Time  `json:"starts_at"`
	EndsAt              time.Time  `json:"ends_at"`
	Capacity            int        `json:"capacity"`
	BookedCount         int        `json:"booked_count"`
	WaitlistLimit       int        `json:"waitlist_limit"`
	Status              Status     `json:"status"`
	CancelReason        string     `json:"cancel_reason"`
	CreatedBy           *int64     `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	// 反范式（JOIN 取）。
	CourseTitle    string `json:"course_title"`
	LocationName   string `json:"location_name"`
	InstructorName string `json:"instructor_name"`
	ResourceName   string `json:"resource_name"` // Batch 12a：绑定资源名，未绑定为空。
}

// CreateInput 创建入参。Capacity <= 0 时容量默认值优先级：绑定资源容量 > course.default_capacity。
type CreateInput struct {
	BrandID             int64
	ActorID             int64
	CourseID            int64
	LocationID          int64
	LocationResourceID  *int64 // Batch 12a：可选绑定资源。
	InstructorProfileID int64
	StartsAt            time.Time
	EndsAt              time.Time
	Capacity            int
	WaitlistLimit       int
}

// ListFilter 列表查询。零值表示不过滤。ScopeLocationIDs 非 nil 时按 data_scope 收紧。
type ListFilter struct {
	BrandID             int64
	LocationID          int64
	CourseID            int64
	InstructorProfileID int64
	Status              string
	From                *time.Time
	To                  *time.Time
	ScopeLocationIDs    []int64
}

// Repository 场次仓储接口。
type Repository interface {
	// Create 单事务：校验 course published + 在 location 可用 + instructor 可排课，
	// 落 scheduled；教练时间冲突（DB EXCLUDE 23P01）→ SESSION_INSTRUCTOR_CONFLICT。
	Create(ctx context.Context, in CreateInput) (*Session, error)
	GetByID(ctx context.Context, brandID, id int64) (*Session, error)
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Session, int64, error)
	// Cancel 仅 scheduled/in_progress 可取消，否则 SESSION_CANCEL_NOT_ALLOWED。
	Cancel(ctx context.Context, brandID, actorID, id int64, reason string) (*Session, error)
}
