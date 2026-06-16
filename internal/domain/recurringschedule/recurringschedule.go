// Package recurringschedule 循环排课领域（recurring_schedules + recurring_schedule_weekdays，Batch 12b）。
//
// 简单周重复（blueprint §10.2）：选周几 + 起止，一次批量生成 N 节 class_session，逐节冲突检查、
// 冲突跳过返清单。生成时区固定 Asia/Shanghai(+08:00)。教练/资源时段冲突由 class_sessions 的
// DB EXCLUDE 约束兜底（23P01），按约束名分流 instructor_conflict / resource_conflict。
package recurringschedule

import (
	"context"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/classsession"
)

// Status 循环排课状态。
type Status string

const (
	StatusActive    Status = "active"
	StatusCancelled Status = "cancelled"
	StatusCompleted Status = "completed"
)

// IsValidStatus 判断输入字符串是否合法状态值。
func IsValidStatus(s string) bool {
	switch Status(s) {
	case StatusActive, StatusCancelled, StatusCompleted:
		return true
	}
	return false
}

// 跳过原因（返给前端展示）。
const (
	SkipInstructorConflict = "instructor_conflict"
	SkipResourceConflict   = "resource_conflict"
)

// Schedule 循环排课实体（含列表/详情用反范式名 + 周几 + 已生成场次数）。
type Schedule struct {
	ID                  int64     `json:"id"`
	BrandID             int64     `json:"brand_id"`
	LocationID          int64     `json:"location_id"`
	LocationResourceID  *int64    `json:"location_resource_id"`
	CourseID            int64     `json:"course_id"`
	InstructorProfileID int64     `json:"instructor_profile_id"`
	Weekdays            []int     `json:"weekdays"`   // 0=周日 … 6=周六（time.Weekday）
	StartDate           string    `json:"start_date"` // YYYY-MM-DD
	EndDate             string    `json:"end_date"`   // YYYY-MM-DD，空串=用 repeat_weeks
	RepeatWeeks         *int      `json:"repeat_weeks"`
	StartTime           string    `json:"start_time"` // HH:mm
	DurationMin         int       `json:"duration_min"`
	Capacity            int       `json:"capacity"`
	Status              Status    `json:"status"`
	CreatedBy           *int64    `json:"created_by"`
	CreatedAt           time.Time `json:"created_at"`
	// 反范式（JOIN 取）。
	LocationName   string `json:"location_name"`
	CourseTitle    string `json:"course_title"`
	InstructorName string `json:"instructor_name"`
	ResourceName   string `json:"resource_name"`
	SessionCount   int64  `json:"session_count"`
}

// Occurrence 一节待生成场次的时刻（UTC）+ 本地展示标签（Asia/Shanghai）。
type Occurrence struct {
	StartsAt  time.Time
	EndsAt    time.Time
	DateLabel string // YYYY-MM-DD（本地）
	TimeLabel string // HH:mm（本地）
}

// SkippedOccurrence 冲突跳过的场次（返清单）。
type SkippedOccurrence struct {
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
	Reason    string `json:"reason"`
}

// GenerateInput 生成入参。Occurrences 由 application 层按时区算好；row 元数据用于落 recurring_schedules。
type GenerateInput struct {
	BrandID             int64
	ActorID             int64
	CourseID            int64
	LocationID          int64
	LocationResourceID  *int64
	InstructorProfileID int64
	Weekdays            []int
	StartDate           string // YYYY-MM-DD
	EndDate             string // YYYY-MM-DD（可空）
	RepeatWeeks         *int
	StartTime           string // HH:mm
	DurationMin         int
	Capacity            int // <=0 时 repo 按 资源容量 > course.default_capacity 取默认
	Occurrences         []Occurrence
}

// GenerateResult 生成结果。
type GenerateResult struct {
	Schedule *Schedule
	Created  []*classsession.Session
	Skipped  []SkippedOccurrence
}

// ListFilter 列表查询。
type ListFilter struct {
	BrandID          int64
	LocationID       int64
	Status           string
	ScopeLocationIDs []int64
}

// Repository 循环排课仓储接口。
type Repository interface {
	// Generate 外层 tx 插 recurring + weekdays，批级校验，逐 occurrence SAVEPOINT 插场次；
	// 冲突跳过记 Skipped；0 成功 → 整批回滚 + RECURRING_ALL_CONFLICT。
	Generate(ctx context.Context, in GenerateInput) (*GenerateResult, error)
	GetByID(ctx context.Context, brandID, id int64) (*Schedule, error)
	// GetDetail 返回模板 + 已生成场次（按 starts_at 升序）。
	GetDetail(ctx context.Context, brandID, id int64) (*Schedule, []*classsession.Session, error)
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Schedule, int64, error)
	// Cancel 仅 active 可取消（status→cancelled），不级联已生成场次。
	Cancel(ctx context.Context, brandID, actorID, id int64) (*Schedule, error)
}
