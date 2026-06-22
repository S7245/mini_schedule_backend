package persistence

import "time"

// WaitlistEntryModel waitlist_entries 表（Batch 13d）。
// skipped_reason 用指针（nullable）；status waiting 时为 NULL。
type WaitlistEntryModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	ClassSessionID        int64     `gorm:"column:class_session_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	Position              int       `gorm:"column:position"`
	Status                string    `gorm:"column:status"`
	PromotedBookingID     *int64    `gorm:"column:promoted_booking_id"`
	SkippedReason         *string   `gorm:"column:skipped_reason"`
	OperatedBy            *int64    `gorm:"column:operated_by"`
}

func (WaitlistEntryModel) TableName() string { return "waitlist_entries" }
