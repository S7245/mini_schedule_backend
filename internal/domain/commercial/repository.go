package commercial

import "context"

type CreateSaaSPlanInput struct {
	Name              string
	Description       string
	MonthlyPrice      string
	YearlyPrice       string
	YearlyDiscountPct *string
	Currency          string
	MaxLocations      int
	MaxStaffSeats     int
	MaxLearners       int
	SortOrder         int
	Features          []SaaSPlanFeatureInput
}

type SaaSPlanFeatureInput struct {
	FeatureCode string
	Enabled     bool
}

type UpdateSaaSPlanInput struct {
	Name              *string
	Description       *string
	MonthlyPrice      *string
	YearlyPrice       *string
	YearlyDiscountPct *string
	Currency          *string
	MaxLocations      *int
	MaxStaffSeats     *int
	MaxLearners       *int
	SortOrder         *int
	Features          *[]SaaSPlanFeatureInput
}

type CreatePublicSignupOrderInput struct {
	Phone          string
	SMSCode        string
	Password       string
	BrandName      string
	LogoURL        string
	ContactName    string
	ContactEmail   string
	IndustryType   string
	PlanID         int64
	BillingCycle   BillingCycle
	PaymentChannel PaymentChannel
}

type CreatePublicSignupOrderRecordInput struct {
	Phone          string
	PasswordHash   string
	BrandName      string
	LogoURL        string
	ContactName    string
	ContactEmail   string
	IndustryType   string
	PlanID         int64
	BillingCycle   BillingCycle
	PaymentChannel PaymentChannel
	OutTradeNo     string
}

type PublicSignupOrderResult struct {
	BrandID         int64          `json:"brand_id"`
	BrandName       string         `json:"brand_name"`
	BrandStatus     string         `json:"brand_status"`
	BrandUserID     int64          `json:"brand_user_id"`
	BrandUserPhone  string         `json:"brand_user_phone"`
	BrandUserStatus string         `json:"brand_user_status"`
	Plan            *SaaSPlan      `json:"plan"`
	Order           *SaaSPlanOrder `json:"order"`
}

type ListSaaSPlanOrdersFilter struct {
	Status         SaaSPlanOrderStatus
	PaymentChannel PaymentChannel
	Source         OrderSource
	BrandID        int64
}

type ListBrandSubscriptionsFilter struct {
	Status  BrandSubscriptionStatus
	BrandID int64
}

type ManualRenewBrandSubscriptionInput struct {
	ActorID      int64
	ExtendMonths int
	ExtendDays   int
	Reason       string
}

type UpdateBrandSubscriptionLimitsInput struct {
	ActorID       int64
	MaxLocations  *int
	MaxStaffSeats *int
	MaxLearners   *int
	Features      *[]SaaSPlanFeatureInput
	Reason        string
}

type UpdateBrandSubscriptionStatusInput struct {
	ActorID      int64
	Status       BrandSubscriptionStatus
	FrozenReason string
	Reason       string
}

type ListPaymentTransactionsFilter struct {
	Status         PaymentTransactionStatus
	PaymentChannel PaymentChannel
	OrderID        int64
	BrandID        int64
}

type ListOperationLogsFilter struct {
	BrandID    int64
	Action     string
	TargetType string
	TargetID   int64
}

type PlatformSummary struct {
	BrandTotal              int64  `json:"brand_total"`
	PendingBrandTotal       int64  `json:"pending_brand_total"`
	ActiveBrandTotal        int64  `json:"active_brand_total"`
	ActiveSubscriptionTotal int64  `json:"active_subscription_total"`
	ExpiringIn7DaysTotal    int64  `json:"expiring_in_7_days_total"`
	RestrictedOrFrozenTotal int64  `json:"restricted_or_frozen_total"`
	TodayOrderTotal         int64  `json:"today_order_total"`
	TodayPaidAmount         string `json:"today_paid_amount"`
	ExceptionOrderTotal     int64  `json:"exception_order_total"`
	FailedCallbackTotal     int64  `json:"failed_callback_total"`
}

type Repository interface {
	CreateSaaSPlan(ctx context.Context, input CreateSaaSPlanInput) (*SaaSPlan, error)
	GetSaaSPlan(ctx context.Context, id int64) (*SaaSPlan, error)
	ListSaaSPlans(ctx context.Context, offset, limit int, includeInactive bool) ([]*SaaSPlan, int64, error)
	ListPublicSaaSPlans(ctx context.Context) ([]*SaaSPlan, error)
	UpdateSaaSPlan(ctx context.Context, id int64, input UpdateSaaSPlanInput) (*SaaSPlan, error)
	UpdateSaaSPlanStatus(ctx context.Context, id int64, status SaaSPlanStatus) error
	CreatePublicSignupOrder(ctx context.Context, input CreatePublicSignupOrderRecordInput) (*PublicSignupOrderResult, error)

	ListSaaSPlanOrders(ctx context.Context, offset, limit int, filter ListSaaSPlanOrdersFilter) ([]*SaaSPlanOrder, int64, error)
	GetBrandSubscription(ctx context.Context, id int64) (*BrandSubscription, error)
	ListBrandSubscriptions(ctx context.Context, offset, limit int, filter ListBrandSubscriptionsFilter) ([]*BrandSubscription, int64, error)
	ManualRenewBrandSubscription(ctx context.Context, id int64, input ManualRenewBrandSubscriptionInput) (*BrandSubscription, error)
	UpdateBrandSubscriptionLimits(ctx context.Context, id int64, input UpdateBrandSubscriptionLimitsInput) (*BrandSubscription, error)
	UpdateBrandSubscriptionStatus(ctx context.Context, id int64, input UpdateBrandSubscriptionStatusInput) (*BrandSubscription, error)
	ListPaymentTransactions(ctx context.Context, offset, limit int, filter ListPaymentTransactionsFilter) ([]*PaymentTransaction, int64, error)
	ListPaymentCallbackLogs(ctx context.Context, offset, limit int, status PaymentCallbackLogStatus) ([]*PaymentCallbackLog, int64, error)
	ListOperationLogs(ctx context.Context, offset, limit int, filter ListOperationLogsFilter) ([]*OperationLog, int64, error)

	GetPlatformSummary(ctx context.Context) (*PlatformSummary, error)
}
