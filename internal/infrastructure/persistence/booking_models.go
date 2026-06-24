package persistence

import "time"

// BookingModel bookings 表（Batch 13c）。
// cancel_source / no_entitlement_reason 用指针：空串会触发 CHECK（cancel_source IN(...) / 占位
// reason NOT NULL）或落非 NULL 脏值，未取消/非占位时须为 NULL。
type BookingModel struct {
	ID                     int64      `gorm:"primaryKey;autoIncrement"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
	UpdatedAt              time.Time  `gorm:"column:updated_at"`
	BrandID                int64      `gorm:"column:brand_id"`
	ClassSessionID         int64      `gorm:"column:class_session_id"`
	BrandLearnerProfileID  int64      `gorm:"column:brand_learner_profile_id"`
	Source                 string     `gorm:"column:source"`
	Status                 string     `gorm:"column:status"`
	BookedAt               time.Time  `gorm:"column:booked_at"`
	CancelledAt            *time.Time `gorm:"column:cancelled_at"`
	CancelledBy            *int64     `gorm:"column:cancelled_by"`
	CancelSource           *string    `gorm:"column:cancel_source"`
	CancelReason           string     `gorm:"column:cancel_reason"`
	AssistedBy             *int64     `gorm:"column:assisted_by"`
	RequiresEntitlementFix bool       `gorm:"column:requires_entitlement_fix"`
	NoEntitlementReason    *string    `gorm:"column:no_entitlement_reason"`
}

func (BookingModel) TableName() string { return "bookings" }

// EntitlementHoldModel entitlement_holds 表（一 booking 一 hold，unique(booking_id)）。
type EntitlementHoldModel struct {
	ID                    int64      `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
	BrandID               int64      `gorm:"column:brand_id"`
	BookingID             int64      `gorm:"column:booking_id"`
	LearnerEntitlementID  int64      `gorm:"column:learner_entitlement_id"`
	BrandLearnerProfileID int64      `gorm:"column:brand_learner_profile_id"`
	Credits               int        `gorm:"column:credits"`
	Status                string     `gorm:"column:status"`
	HeldAt                time.Time  `gorm:"column:held_at"`
	ReleasedAt            *time.Time `gorm:"column:released_at"`
	ConsumedAt            *time.Time `gorm:"column:consumed_at"`
}

func (EntitlementHoldModel) TableName() string { return "entitlement_holds" }

// BrandBookingPolicyModel brand_booking_policies 表。location_id NULL = brand-default 行。
// 无 monthly 列（月限独家来自 entitlement_products）。
type BrandBookingPolicyModel struct {
	ID                        int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt                 time.Time `gorm:"column:created_at"`
	UpdatedAt                 time.Time `gorm:"column:updated_at"`
	BrandID                   int64     `gorm:"column:brand_id"`
	LocationID                *int64    `gorm:"column:location_id"`
	BookAheadMaxMinutes       *int      `gorm:"column:book_ahead_max_minutes"`
	BookAheadMinMinutes       int       `gorm:"column:book_ahead_min_minutes"`
	CancelDeadlineMinutes     int       `gorm:"column:cancel_deadline_minutes"`
	ReleaseOnCancel           bool      `gorm:"column:release_on_cancel"`
	NoShowConsumesEntitlement bool      `gorm:"column:no_show_consumes_entitlement"`
	DailyBookingLimit         *int      `gorm:"column:daily_booking_limit"`
	WeeklyBookingLimit        *int      `gorm:"column:weekly_booking_limit"`
	ConcurrentBookingLimit    *int      `gorm:"column:concurrent_booking_limit"`
	AllowWaitlist             bool      `gorm:"column:allow_waitlist"`
	WaitlistLimit             int       `gorm:"column:waitlist_limit"`
}

func (BrandBookingPolicyModel) TableName() string { return "brand_booking_policies" }

// ClassSessionPolicyOverrideModel class_session_policy_overrides 表（稀疏覆盖，nil=继承）。
type ClassSessionPolicyOverrideModel struct {
	ID                        int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt                 time.Time `gorm:"column:created_at"`
	UpdatedAt                 time.Time `gorm:"column:updated_at"`
	BrandID                   int64     `gorm:"column:brand_id"`
	ClassSessionID            int64     `gorm:"column:class_session_id"`
	AllowCancel               *bool     `gorm:"column:allow_cancel"`
	CancelDeadlineMinutes     *int      `gorm:"column:cancel_deadline_minutes"`
	ReleaseOnCancel           *bool     `gorm:"column:release_on_cancel"`
	NoShowConsumesEntitlement *bool     `gorm:"column:no_show_consumes_entitlement"`
	AllowWaitlist             *bool     `gorm:"column:allow_waitlist"`
	WaitlistLimit             *int      `gorm:"column:waitlist_limit"`
}

func (ClassSessionPolicyOverrideModel) TableName() string { return "class_session_policy_overrides" }

// AttendanceRecordModel attendance_records 表（Batch 13e）。unique(booking_id) 防重签。
type AttendanceRecordModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	BookingID             int64     `gorm:"column:booking_id"`
	ClassSessionID        int64     `gorm:"column:class_session_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	MarkedBy              *int64    `gorm:"column:marked_by"`
	AttendedAt            time.Time `gorm:"column:attended_at"`
	Note                  *string   `gorm:"column:note"`
}

func (AttendanceRecordModel) TableName() string { return "attendance_records" }

// EntitlementConsumptionModel entitlement_consumptions 表（Batch 13e）。
// unique(entitlement_hold_id) WHERE NOT NULL 防重复消费同 hold。consumption_type ∈ {attendance,no_show,manual}。
type EntitlementConsumptionModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	EntitlementHoldID     *int64    `gorm:"column:entitlement_hold_id"`
	LearnerEntitlementID  int64     `gorm:"column:learner_entitlement_id"`
	BookingID             int64     `gorm:"column:booking_id"`
	AttendanceID          *int64    `gorm:"column:attendance_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	Credits               int       `gorm:"column:credits"`
	ConsumptionType       string    `gorm:"column:consumption_type"`
	ConsumedAt            time.Time `gorm:"column:consumed_at"`
	OperatedBy            *int64    `gorm:"column:operated_by"`
	Note                  *string   `gorm:"column:note"`
}

func (EntitlementConsumptionModel) TableName() string { return "entitlement_consumptions" }

// SessionRecordModel session_records 表（Batch 13e，履约记录）。record_type ∈ {attendance,no_show,manual}。
// metadata 列省略不映射 → INSERT 不带，DB 默认 '{}' 生效（v1 不承载富履约数据）。
type SessionRecordModel struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	BrandID               int64     `gorm:"column:brand_id"`
	ClassSessionID        int64     `gorm:"column:class_session_id"`
	BookingID             int64     `gorm:"column:booking_id"`
	AttendanceID          *int64    `gorm:"column:attendance_id"`
	BrandLearnerProfileID int64     `gorm:"column:brand_learner_profile_id"`
	InstructorProfileID   *int64    `gorm:"column:instructor_profile_id"`
	RecordType            string    `gorm:"column:record_type"`
	Note                  *string   `gorm:"column:note"`
}

func (SessionRecordModel) TableName() string { return "session_records" }
