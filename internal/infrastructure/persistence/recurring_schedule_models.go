package persistence

import "time"

// RecurringScheduleModel recurring_schedules 表（Batch 12b）。无软删列。
// 日期/时间列用 string + PG 文本→date/time 赋值转换插入；读取走 to_char 投影（见 repo）。
type RecurringScheduleModel struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
	BrandID             int64     `gorm:"column:brand_id"`
	LocationID          int64     `gorm:"column:location_id"`
	LocationResourceID  *int64    `gorm:"column:location_resource_id"`
	CourseID            int64     `gorm:"column:course_id"`
	InstructorProfileID int64     `gorm:"column:instructor_profile_id"`
	StartDate           string    `gorm:"column:start_date;type:date"`
	EndDate             *string   `gorm:"column:end_date;type:date"`
	RepeatWeeks         *int      `gorm:"column:repeat_weeks"`
	StartTime           string    `gorm:"column:start_time;type:time"`
	DurationMin         int       `gorm:"column:duration_min"`
	Capacity            int       `gorm:"column:capacity"`
	Status              string    `gorm:"column:status"`
	CreatedBy           *int64    `gorm:"column:created_by"`
}

func (RecurringScheduleModel) TableName() string { return "recurring_schedules" }

// RecurringScheduleWeekdayModel recurring_schedule_weekdays 表（weekday 0=周日..6=周六）。
type RecurringScheduleWeekdayModel struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	RecurringScheduleID int64     `gorm:"column:recurring_schedule_id"`
	Weekday             int       `gorm:"column:weekday"`
}

func (RecurringScheduleWeekdayModel) TableName() string { return "recurring_schedule_weekdays" }
