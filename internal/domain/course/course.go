package course

import "time"

// Course 课程/计划实体
type Course struct {
	ID          int64      `json:"id"`
	BrandID     int64      `json:"brand_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	CoverURL    string     `json:"cover_url"`
	Difficulty  Difficulty `json:"difficulty"`
	DurationMin int        `json:"duration_min"` // 预计时长（分钟）
	Type        CourseType `json:"type"`
	Status      Status     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Difficulty string

const (
	DifficultyBeginner   Difficulty = "beginner"
	DifficultyIntermediate Difficulty = "intermediate"
	DifficultyAdvanced   Difficulty = "advanced"
)

type CourseType string

const (
	TypeStrength    CourseType = "strength"    // 力量训练
	TypeCardio      CourseType = "cardio"      // 有氧训练
	TypeFlexibility CourseType = "flexibility" // 柔韧性训练
	TypeHIIT        CourseType = "hiit"        // 高强度间歇
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

// CreateCourseInput 创建课程输入
type CreateCourseInput struct {
	BrandID     int64      `validate:"required,gt=0"`
	Title       string     `validate:"required,min=2,max=200"`
	Description string     `validate:"omitempty,max=2000"`
	CoverURL    string     `validate:"omitempty,url"`
	Difficulty  Difficulty `validate:"required,oneof=beginner intermediate advanced"`
	DurationMin int        `validate:"required,gt=0"`
	Type        CourseType `validate:"required,oneof=strength cardio flexibility hiit"`
}

// UpdateCourseInput 更新课程输入
type UpdateCourseInput struct {
	Title       *string     `validate:"omitempty,min=2,max=200"`
	Description *string     `validate:"omitempty,max=2000"`
	CoverURL    *string     `validate:"omitempty,url"`
	Difficulty  *Difficulty `validate:"omitempty,oneof=beginner intermediate advanced"`
	DurationMin *int        `validate:"omitempty,gt=0"`
	Type        *CourseType `validate:"omitempty,oneof=strength cardio flexibility hiit"`
}
