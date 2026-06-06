package persistence

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel GORM 基础模型，所有实体继承
type BaseModel struct {
	ID        int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BrandModel 品牌 GORM 模型
type BrandModel struct {
	BaseModel
	Name                  string     `gorm:"size:100;not null;index" json:"name"`
	LogoURL               string     `gorm:"size:500" json:"logo_url"`
	ContactName           string     `gorm:"size:50;not null" json:"contact_name"`
	ContactPhone          string     `gorm:"size:20;not null;uniqueIndex" json:"contact_phone"`
	ContactEmail          string     `gorm:"size:100" json:"contact_email"`
	BrandCode             string     `gorm:"size:50" json:"brand_code"`
	IndustryType          string     `gorm:"size:50" json:"industry_type"`
	Description           string     `gorm:"size:2000" json:"description"`
	OnboardingStatus      string     `gorm:"size:30;not null;default:not_started" json:"onboarding_status"`
	OnboardingCompletedAt *time.Time `json:"onboarding_completed_at"`
	Status                string     `gorm:"size:20;not null;default:pending;index" json:"status"`
}

func (BrandModel) TableName() string { return "brands" }

// BrandUserModel 品牌管理员 GORM 模型
type BrandUserModel struct {
	BaseModel
	BrandID      int64  `gorm:"not null;index" json:"brand_id"`
	Phone        string `gorm:"size:20;not null;uniqueIndex" json:"phone"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	Name         string `gorm:"size:50;not null" json:"name"`
	Status       string `gorm:"size:20;not null;default:active" json:"status"`
}

func (BrandUserModel) TableName() string { return "brand_users" }

// AppUserModel C 端用户 GORM 模型
type AppUserModel struct {
	BaseModel
	BrandID   int64  `gorm:"not null;index" json:"brand_id"`
	OpenID    string `gorm:"size:100;not null;uniqueIndex:idx_brand_openid" json:"openid"`
	Phone     string `gorm:"size:20;index" json:"phone"`
	Nickname  string `gorm:"size:50" json:"nickname"`
	AvatarURL string `gorm:"size:500" json:"avatar_url"`
	VIPLevel  string `gorm:"size:20;not null;default:free" json:"vip_level"`
	Status    string `gorm:"size:20;not null;default:active" json:"status"`
}

func (AppUserModel) TableName() string { return "app_users" }

// AdminUserModel 平台管理员 GORM 模型
type AdminUserModel struct {
	BaseModel
	Username     string `gorm:"size:50;not null;uniqueIndex" json:"username"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	Role         string `gorm:"size:20;not null;default:operator" json:"role"`
	Status       string `gorm:"size:20;not null;default:active" json:"status"`
}

func (AdminUserModel) TableName() string { return "admin_users" }

// CourseModel 课程 GORM 模型
type CourseModel struct {
	BaseModel
	BrandID     int64  `gorm:"not null;index" json:"brand_id"`
	Title       string `gorm:"size:200;not null;index" json:"title"`
	Description string `gorm:"size:2000" json:"description"`
	CoverURL    string `gorm:"size:500" json:"cover_url"`
	Difficulty  string `gorm:"size:20;not null" json:"difficulty"`
	DurationMin int    `gorm:"not null" json:"duration_min"`
	Type        string `gorm:"size:20;not null;index" json:"type"`
	Status      string `gorm:"size:20;not null;default:draft;index" json:"status"`
}

func (CourseModel) TableName() string { return "courses" }

// TrainingRecordModel 训练记录 GORM 模型
type TrainingRecordModel struct {
	BaseModel
	UserID      int64   `gorm:"not null;index" json:"user_id"`
	BrandID     int64   `gorm:"not null;index" json:"brand_id"`
	CourseID    int64   `gorm:"not null;index" json:"course_id"`
	DurationMin int     `gorm:"not null" json:"duration_min"`
	Calories    float64 `gorm:"not null;default:0" json:"calories"`
	Notes       string  `gorm:"size:500" json:"notes"`
	CompletedAt time.Time `gorm:"not null;index" json:"completed_at"`
}

func (TrainingRecordModel) TableName() string { return "training_records" }
