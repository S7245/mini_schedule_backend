package persistence

import (
	"time"

	"gorm.io/gorm"
)

// LearnerIdentityModel learner_identities 表（跨品牌复用的学员身份，Batch 13a）。
// 无微信学员由员工创建时用合成 wechat_open_id 占位；phone 全局 partial-unique，按手机号对账。
type LearnerIdentityModel struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
	WechatOpenID  string    `gorm:"column:wechat_open_id"`
	WechatUnionID *string   `gorm:"column:wechat_union_id"`
	Phone         *string   `gorm:"column:phone"`
	Nickname      string    `gorm:"column:nickname"`
	AvatarURL     string    `gorm:"column:avatar_url"`
	Status        string    `gorm:"column:status"`
}

func (LearnerIdentityModel) TableName() string { return "learner_identities" }

// BrandLearnerProfileModel brand_learner_profiles 表。软删走 gorm.DeletedAt。
type BrandLearnerProfileModel struct {
	ID                int64          `gorm:"primaryKey;autoIncrement"`
	CreatedAt         time.Time      `gorm:"column:created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index"`
	BrandID           int64          `gorm:"column:brand_id"`
	LearnerIdentityID int64          `gorm:"column:learner_identity_id"`
	PrimaryLocationID *int64         `gorm:"column:primary_location_id"`
	LearnerNo         *string        `gorm:"column:learner_no"`
	Nickname          string         `gorm:"column:nickname"`
	Remark            string         `gorm:"column:remark"`
	Status            string         `gorm:"column:status"`
}

func (BrandLearnerProfileModel) TableName() string { return "brand_learner_profiles" }

// LearnerTagModel learner_tags 表（品牌级，无 location scope）。
type LearnerTagModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
	BrandID   int64     `gorm:"column:brand_id"`
	Name      string    `gorm:"column:name"`
	Color     string    `gorm:"column:color"`
	Status    string    `gorm:"column:status"`
}

func (LearnerTagModel) TableName() string { return "learner_tags" }

// LearnerTagAssignmentModel learner_tag_assignments 表（学员↔标签，硬删重插）。
type LearnerTagAssignmentModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	TagID                 int64     `gorm:"column:tag_id"`
}

func (LearnerTagAssignmentModel) TableName() string { return "learner_tag_assignments" }
