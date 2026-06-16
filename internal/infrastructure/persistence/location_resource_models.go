package persistence

import (
	"time"

	"gorm.io/gorm"
)

// LocationResourceModel location_resources 表（Batch 12a）。软删走 gorm.DeletedAt。
type LocationResourceModel struct {
	ID         int64          `gorm:"primaryKey;autoIncrement"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index"`
	BrandID    int64          `gorm:"column:brand_id"`
	LocationID int64          `gorm:"column:location_id"`
	Name       string         `gorm:"column:name"`
	Type       string         `gorm:"column:type"`
	Capacity   int            `gorm:"column:capacity"`
	Status     string         `gorm:"column:status"`
	Remark     string         `gorm:"column:remark"`
}

func (LocationResourceModel) TableName() string { return "location_resources" }
