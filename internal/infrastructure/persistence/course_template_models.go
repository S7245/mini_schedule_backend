package persistence

import "time"

// CourseCategoryModel course_categories 表（Batch 11）。
type CourseCategoryModel struct {
	ID                int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	BrandID           int64  `gorm:"column:brand_id"`
	Name              string `gorm:"column:name"`
	Color             string `gorm:"column:color"`
	Icon              string `gorm:"column:icon"`
	SortOrder         int    `gorm:"column:sort_order"`
	ShowInMiniProgram bool   `gorm:"column:show_in_mini_program"`
	Status            string `gorm:"column:status"`
}

func (CourseCategoryModel) TableName() string { return "course_categories" }

// CourseTemplateModel courses 表（Batch 11 视角，含 default_capacity / level_label /
// show_in_mini_program / published_at 等 000003 加的列；与 legacy CourseModel 区分语义，
// 单独建模以承载新字段）。
type CourseTemplateModel struct {
	ID                int64      `gorm:"primaryKey;autoIncrement"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
	DeletedAt         *time.Time `gorm:"column:deleted_at"`
	BrandID           int64      `gorm:"column:brand_id"`
	Title             string     `gorm:"column:title"`
	Description       string     `gorm:"column:description"`
	CoverURL          string     `gorm:"column:cover_url"`
	LevelLabel        string     `gorm:"column:level_label"`
	DurationMin       int        `gorm:"column:duration_min"`
	DefaultCapacity   int        `gorm:"column:default_capacity"`
	ShowInMiniProgram bool       `gorm:"column:show_in_mini_program"`
	Status            string     `gorm:"column:status"`
	PublishedAt       *time.Time `gorm:"column:published_at"`
}

func (CourseTemplateModel) TableName() string { return "courses" }

// CourseCategoryAssignmentModel course_category_assignments（课程↔分类多对多）。
type CourseCategoryAssignmentModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	BrandID    int64     `gorm:"column:brand_id"`
	CourseID   int64     `gorm:"column:course_id"`
	CategoryID int64     `gorm:"column:category_id"`
}

func (CourseCategoryAssignmentModel) TableName() string { return "course_category_assignments" }

// CourseLocationAvailabilityModel course_location_availability（课程↔可用门店）。
type CourseLocationAvailabilityModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
	BrandID     int64     `gorm:"column:brand_id"`
	CourseID    int64     `gorm:"column:course_id"`
	LocationID  int64     `gorm:"column:location_id"`
	IsAvailable bool      `gorm:"column:is_available"`
	Note        string    `gorm:"column:note"`
}

func (CourseLocationAvailabilityModel) TableName() string { return "course_location_availability" }

// ClassSessionModel class_sessions 表（Batch 11 单场次）。
type ClassSessionModel struct {
	ID                  int64      `gorm:"primaryKey;autoIncrement"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	BrandID             int64      `gorm:"column:brand_id"`
	LocationID          int64      `gorm:"column:location_id"`
	LocationResourceID  *int64     `gorm:"column:location_resource_id"`
	CourseID            int64      `gorm:"column:course_id"`
	InstructorProfileID int64      `gorm:"column:instructor_profile_id"`
	RecurringScheduleID *int64     `gorm:"column:recurring_schedule_id"`
	StartsAt            time.Time  `gorm:"column:starts_at"`
	EndsAt              time.Time  `gorm:"column:ends_at"`
	Capacity            int        `gorm:"column:capacity"`
	BookedCount         int        `gorm:"column:booked_count"`
	WaitlistLimit       int        `gorm:"column:waitlist_limit"`
	Status              string     `gorm:"column:status"`
	CancelReason        string     `gorm:"column:cancel_reason"`
	CreatedBy           *int64     `gorm:"column:created_by"`
}

func (ClassSessionModel) TableName() string { return "class_sessions" }
