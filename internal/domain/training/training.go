package training

import "time"

// Record 训练记录实体
type Record struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	BrandID     int64     `json:"brand_id"`
	CourseID    int64     `json:"course_id"`
	DurationMin int       `json:"duration_min"` // 实际训练时长（分钟）
	Calories    float64   `json:"calories"`     // 消耗卡路里
	Notes       string    `json:"notes"`
	CompletedAt time.Time `json:"completed_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateRecordInput 创建训练记录输入
type CreateRecordInput struct {
	UserID      int64   `validate:"required,gt=0"`
	BrandID     int64   `validate:"required,gt=0"`
	CourseID    int64   `validate:"required,gt=0"`
	DurationMin int     `validate:"required,gt=0"`
	Calories    float64 `validate:"omitempty,gte=0"`
	Notes       string  `validate:"omitempty,max=500"`
}
