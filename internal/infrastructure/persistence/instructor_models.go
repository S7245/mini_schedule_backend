package persistence

import "time"

// InstructorProfileModel instructor_profiles 表，1:1 关联 brand_users。
type InstructorProfileModel struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
	BrandID             int64  `gorm:"not null;index"`
	BrandUserID         int64  `gorm:"not null;uniqueIndex"`
	DisplayName         string `gorm:"size:100;not null"`
	AvatarURL           string `gorm:"size:500"`
	Bio                 string `gorm:"size:2000"`
	Specialties         string `gorm:"size:1000"`
	Certificates        string `gorm:"size:1000"`
	IsVisibleToLearners bool   `gorm:"not null;default:true"`
	IsSchedulable       bool   `gorm:"not null;default:true"`
	Status              string `gorm:"size:20;not null;default:active"`
}

func (InstructorProfileModel) TableName() string { return "instructor_profiles" }
