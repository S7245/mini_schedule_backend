package persistence

import "time"

// EntitlementProductModel entitlement_products 表（Batch 13b）。无 deleted_at，仅 status 启停。
type EntitlementProductModel struct {
	ID                     int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt              time.Time `gorm:"column:created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at"`
	BrandID                int64     `gorm:"column:brand_id"`
	Name                   string    `gorm:"column:name"`
	Description            string    `gorm:"column:description"`
	ProductType            string    `gorm:"column:product_type"`
	TotalCredits           *int      `gorm:"column:total_credits"`
	ValidityDays           int       `gorm:"column:validity_days"`
	DailyBookingLimit      *int      `gorm:"column:daily_booking_limit"`
	WeeklyBookingLimit     *int      `gorm:"column:weekly_booking_limit"`
	MonthlyBookingLimit    *int      `gorm:"column:monthly_booking_limit"`
	ConcurrentBookingLimit *int      `gorm:"column:concurrent_booking_limit"`
	LocationScope          string    `gorm:"column:location_scope"`
	CourseScope            string    `gorm:"column:course_scope"`
	Status                 string    `gorm:"column:status"`
}

func (EntitlementProductModel) TableName() string { return "entitlement_products" }

// EntitlementProductLocationModel 产品↔门店 scope 关联（specific 时使用，硬删重插）。
type EntitlementProductLocationModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	BrandID    int64     `gorm:"column:brand_id"`
	ProductID  int64     `gorm:"column:product_id"`
	LocationID int64     `gorm:"column:location_id"`
}

func (EntitlementProductLocationModel) TableName() string { return "entitlement_product_locations" }

// EntitlementProductCourseModel 产品↔课程 scope 关联（specific 时使用，硬删重插）。
type EntitlementProductCourseModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"column:created_at"`
	BrandID   int64     `gorm:"column:brand_id"`
	ProductID int64     `gorm:"column:product_id"`
	CourseID  int64     `gorm:"column:course_id"`
}

func (EntitlementProductCourseModel) TableName() string { return "entitlement_product_courses" }

// LearnerEntitlementModel learner_entitlements 表。
type LearnerEntitlementModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	ProductID             int64     `gorm:"column:product_id"`
	Status                string    `gorm:"column:status"`
	TotalCredits          *int      `gorm:"column:total_credits"`
	RemainingCredits      *int      `gorm:"column:remaining_credits"`
	LockedCredits         int       `gorm:"column:locked_credits"`
	ConsumedCredits       int       `gorm:"column:consumed_credits"`
	StartsAt              time.Time `gorm:"column:starts_at"`
	ExpiresAt             time.Time `gorm:"column:expires_at"`
	GrantedBy             *int64    `gorm:"column:granted_by"`
	Remark                string    `gorm:"column:remark"`
}

func (LearnerEntitlementModel) TableName() string { return "learner_entitlements" }

// EntitlementTransactionModel entitlement_transactions 表（流水账）。
type EntitlementTransactionModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	LearnerEntitlementID  int64     `gorm:"column:learner_entitlement_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	BookingID             *int64    `gorm:"column:booking_id"`
	HoldID                *int64    `gorm:"column:hold_id"`
	ConsumptionID         *int64    `gorm:"column:consumption_id"`
	Action                string    `gorm:"column:action"`
	DeltaCredits          int       `gorm:"column:delta_credits"`
	BalanceAfter          *int      `gorm:"column:balance_after"`
	Note                  string    `gorm:"column:note"`
	OperatedBy            *int64    `gorm:"column:operated_by"`
}

func (EntitlementTransactionModel) TableName() string { return "entitlement_transactions" }
