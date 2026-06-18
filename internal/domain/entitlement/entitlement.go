// Package entitlement 权益领域（entitlement_products / learner_entitlements /
// entitlement_transactions，Batch 13b）。
//
// EntitlementProduct 是品牌定义的权益模板（次数/课时包、单次体验包、基础会员卡）；
// LearnerEntitlement 是发放给某学员的持有权益（lock→consume 模型，本批只做 grant + 手动调整，
// locked/consumed 留 13c hold / 13e consume）。EntitlementTransaction 是权益流水账。
package entitlement

import (
	"context"
	"time"
)

// ProductType 权益产品类型（与 DB CHECK entitlement_products_type_valid 对齐）。
type ProductType string

const (
	ProductClassPack      ProductType = "class_pack"
	ProductTrialPack      ProductType = "trial_pack"
	ProductMembershipCard ProductType = "membership_card"
)

// IsValidProductType 判断输入是否合法产品类型。
func IsValidProductType(s string) bool {
	switch ProductType(s) {
	case ProductClassPack, ProductTrialPack, ProductMembershipCard:
		return true
	}
	return false
}

// IsCountBased 课包/体验包按次数计；会员卡不限次（total_credits NULL）。
func IsCountBased(t ProductType) bool {
	return t == ProductClassPack || t == ProductTrialPack
}

// ProductStatus 产品状态（无软删，仅启停）。
type ProductStatus string

const (
	ProductStatusActive   ProductStatus = "active"
	ProductStatusInactive ProductStatus = "inactive"
)

// IsValidProductStatus 判断输入是否合法产品状态。
func IsValidProductStatus(s string) bool {
	return s == string(ProductStatusActive) || s == string(ProductStatusInactive)
}

// Scope 适用范围（与 DB CHECK entitlement_products_scope_valid 对齐）。
const (
	ScopeAll      = "all"
	ScopeSpecific = "specific"
)

// IsValidScope 判断输入是否合法 scope。
func IsValidScope(s string) bool {
	return s == ScopeAll || s == ScopeSpecific
}

// Status 学员权益状态（与 DB CHECK learner_entitlements_status_valid 对齐）。
// active/frozen/cancelled 手动；expired/depleted 派生但落库。
type Status string

const (
	StatusActive    Status = "active"
	StatusExpired   Status = "expired"
	StatusDepleted  Status = "depleted"
	StatusFrozen    Status = "frozen"
	StatusCancelled Status = "cancelled"
)

// Action 流水动作（与 DB CHECK entitlement_transactions_action_valid 对齐）。
// 本批只产生 grant / manual_adjust；hold/release/consume/no_show_consume 留 13c/13e。
type Action string

const (
	ActionGrant         Action = "grant"
	ActionHold          Action = "hold"
	ActionRelease       Action = "release"
	ActionConsume       Action = "consume"
	ActionNoShowConsume Action = "no_show_consume"
	ActionManualAdjust  Action = "manual_adjust"
)

// SettleStatus 纯函数：把 active/expired/depleted 这组派生态按到期/余额结算；frozen/cancelled
// 是手动态保持不变。grant/adjust/reactivate/读时 sweep 共用此逻辑（13c hold 也将复用）。
func SettleStatus(current Status, expiresAt time.Time, totalCredits, remainingCredits *int, now time.Time) Status {
	if current == StatusFrozen || current == StatusCancelled {
		return current
	}
	if !expiresAt.After(now) { // expires_at <= now
		return StatusExpired
	}
	if totalCredits != nil && remainingCredits != nil && *remainingCredits <= 0 {
		return StatusDepleted
	}
	return StatusActive
}

// Product 权益产品（含反范式 issued_count + scope ids）。
type Product struct {
	ID                     int64         `json:"id"`
	BrandID                int64         `json:"brand_id"`
	Name                   string        `json:"name"`
	Description            string        `json:"description"`
	ProductType            ProductType   `json:"product_type"`
	TotalCredits           *int          `json:"total_credits"`
	ValidityDays           int           `json:"validity_days"`
	DailyBookingLimit      *int          `json:"daily_booking_limit"`
	WeeklyBookingLimit     *int          `json:"weekly_booking_limit"`
	MonthlyBookingLimit    *int          `json:"monthly_booking_limit"`
	ConcurrentBookingLimit *int          `json:"concurrent_booking_limit"`
	LocationScope          string        `json:"location_scope"`
	CourseScope            string        `json:"course_scope"`
	Status                 ProductStatus `json:"status"`
	IssuedCount            int           `json:"issued_count"`
	LocationIDs            []int64       `json:"location_ids"`
	CourseIDs              []int64       `json:"course_ids"`
	CreatedAt              time.Time     `json:"created_at"`
	UpdatedAt              time.Time     `json:"updated_at"`
}

// Entitlement 学员持有权益（含反范式 product_name/product_type）。
type Entitlement struct {
	ID                    int64       `json:"id"`
	BrandID               int64       `json:"brand_id"`
	BrandLearnerProfileID int64       `json:"brand_learner_profile_id"`
	ProductID             int64       `json:"product_id"`
	ProductName           string      `json:"product_name"`
	ProductType           ProductType `json:"product_type"`
	Status                Status      `json:"status"`
	TotalCredits          *int        `json:"total_credits"`
	RemainingCredits      *int        `json:"remaining_credits"`
	LockedCredits         int         `json:"locked_credits"`
	ConsumedCredits       int         `json:"consumed_credits"`
	StartsAt              time.Time   `json:"starts_at"`
	ExpiresAt             time.Time   `json:"expires_at"`
	GrantedBy             *int64      `json:"granted_by"`
	Remark                string      `json:"remark"`
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`
}

// Transaction 权益流水。
type Transaction struct {
	ID           int64     `json:"id"`
	Action       Action    `json:"action"`
	DeltaCredits int       `json:"delta_credits"`
	BalanceAfter *int      `json:"balance_after"`
	Note         string    `json:"note"`
	OperatedBy   *int64    `json:"operated_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateProductInput 创建产品入参。限额字段 0 = 不限（存 NULL）；TotalCredits 仅 count-based 用。
type CreateProductInput struct {
	BrandID                int64
	ActorID                int64
	Name                   string
	Description            string
	ProductType            string
	TotalCredits           int
	ValidityDays           int
	DailyBookingLimit      int
	WeeklyBookingLimit     int
	MonthlyBookingLimit    int
	ConcurrentBookingLimit int
	LocationScope          string
	CourseScope            string
	LocationIDs            []int64
	CourseIDs              []int64
}

// UpdateProductInput 更新产品入参（白名单，product_type 不可改）。指针 nil = 不改；
// 限额指针值 0 = 设为不限(NULL)，>0 = 设值。
type UpdateProductInput struct {
	Name                   *string
	Description            *string
	TotalCredits           *int
	ValidityDays           *int
	DailyBookingLimit      *int
	WeeklyBookingLimit     *int
	MonthlyBookingLimit    *int
	ConcurrentBookingLimit *int
	LocationScope          *string
	CourseScope            *string
	LocationIDs            *[]int64
	CourseIDs              *[]int64
}

// ProductListFilter 产品列表查询。
type ProductListFilter struct {
	BrandID     int64
	Status      string
	ProductType string
}

// GrantInput 发放入参。StartsAt 为零值时 repo 取 now。
type GrantInput struct {
	BrandID   int64
	ActorID   int64
	LearnerID int64
	ProductID int64
	StartsAt  *time.Time
	Remark    string
}

// AdjustInput 手动额度调整入参。
type AdjustInput struct {
	BrandID       int64
	ActorID       int64
	EntitlementID int64
	Delta         int
	Reason        string
}

// Repository 权益仓储接口。
type Repository interface {
	CreateProduct(ctx context.Context, in CreateProductInput) (*Product, error)
	GetProduct(ctx context.Context, brandID, id int64) (*Product, error)
	ListProducts(ctx context.Context, filter ProductListFilter, offset, limit int) ([]*Product, int64, error)
	UpdateProduct(ctx context.Context, brandID, actorID, id int64, in UpdateProductInput) (*Product, error)
	UpdateProductStatus(ctx context.Context, brandID, actorID, id int64, status string) (*Product, error)

	// Grant 校验产品 active + 学员属本 brand，快照额度，算 expires，落权益 + grant 流水 + audit。
	Grant(ctx context.Context, in GrantInput) (*Entitlement, error)
	// ListEntitlementsByLearner 先跑 settle sweep（active→expired/depleted 落库）再返。
	ListEntitlementsByLearner(ctx context.Context, brandID, learnerID int64) ([]*Entitlement, error)
	GetEntitlement(ctx context.Context, brandID, id int64) (*Entitlement, error)
	// Adjust SELECT FOR UPDATE + remaining±delta（<0→INSUFFICIENT；不限次卡拒）+ settle + 流水 + audit。
	Adjust(ctx context.Context, in AdjustInput) (*Entitlement, error)
	// SetEntitlementStatus freeze/cancel/reactivate（cancelled 终态不可再变）+ settle + audit。
	SetEntitlementStatus(ctx context.Context, brandID, actorID, id int64, status, reason string) (*Entitlement, error)
	ListTransactions(ctx context.Context, brandID, entitlementID int64) ([]*Transaction, error)
}
