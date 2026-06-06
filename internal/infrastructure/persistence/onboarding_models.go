package persistence

import (
	"time"

	"gorm.io/gorm"
)

// BrandOnboardingStepModel 是 brand_onboarding_steps 表的 GORM 模型。
//
// metadata 是 JSONB NOT NULL DEFAULT '{}'，应用层未赋值时 BeforeCreate 兜底填充
// 空对象，避免 23502。同 Batch 3 PaymentCallbackLogModel 处理。
type BrandOnboardingStepModel struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	BrandID     int64      `gorm:"not null;index" json:"brand_id"`
	StepKey     string     `gorm:"size:80;not null" json:"step_key"`
	Status      string     `gorm:"size:30;not null;default:not_started" json:"status"`
	CompletedAt *time.Time `json:"completed_at"`
	SkippedAt   *time.Time `json:"skipped_at"`
	Metadata    []byte     `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
}

func (BrandOnboardingStepModel) TableName() string { return "brand_onboarding_steps" }

// BeforeCreate 兜底 JSONB NOT NULL 列，防止 nil 切片 → SQL NULL → 23502。
func (m *BrandOnboardingStepModel) BeforeCreate(*gorm.DB) error {
	if len(m.Metadata) == 0 {
		m.Metadata = []byte("{}")
	}
	return nil
}

// LocationModel 是 locations 表的 GORM 模型。
type LocationModel struct {
	ID        int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	BrandID   int64          `gorm:"not null;index" json:"brand_id"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	Address   string         `gorm:"size:500" json:"address"`
	Phone     string         `gorm:"size:20" json:"phone"`
	Status    string         `gorm:"size:20;not null;default:active" json:"status"`
	Remark    string         `gorm:"size:1000" json:"remark"`
}

func (LocationModel) TableName() string { return "locations" }
