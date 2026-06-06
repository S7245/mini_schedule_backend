package commercial

import (
	"context"
	"time"
)

type SaaSPlanOrderStatusResult struct {
	Status string     `json:"status"`
	PaidAt *time.Time `json:"paid_at"`
}

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

// ProcessWeChatCallbackInput 是经验签解密之后，repository 处理订单状态机所需要的全部入参。
type ProcessWeChatCallbackInput struct {
	OutTradeNo        string
	ThirdPartyTradeNo string
	Amount            int64 // 分（cents）
	Currency          string
	TradeState        WeChatTradeState
	PaymentChannel    PaymentChannel
	CallbackRequestID string
	SuccessTime       *time.Time
	ReceivedAt        time.Time
	RawPayload        string // 原始 body 留档
}

// ProcessWeChatCallbackResult 描述本次回调处理后的结果。
//
// 对于幂等 / 忽略 / 失败场景，Success 仍可能为 false，但 HTTP 层依旧需要按
// 微信要求返回 200（除非验签失败）。CallbackLogStatus 用于上层根据情况补
// 写 CallbackLog（对于发生事务回滚的失败场景，CallbackLog 需要事务外写入）。
type ProcessWeChatCallbackResult struct {
	Success           bool
	OrderID           int64
	BrandID           int64
	SubscriptionID    int64
	Message           string
	CallbackLogStatus PaymentCallbackLogStatus
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

	CreateWeChatNativePayOrder(ctx context.Context, orderID int64, codeURL, prepayID string, expiresAt time.Time) error
	GetSaaSPlanOrderStatus(ctx context.Context, orderID int64) (*SaaSPlanOrderStatusResult, error)

	// ProcessWeChatCallback 在单个事务内完成订单状态机推进 + 流水/订阅落库。
	// 对于 trade_state 非 SUCCESS、金额不一致、订单状态不允许处理等业务失败，
	// 不返回 error，而是通过 Result.CallbackLogStatus 告知上层结果。
	// 返回 error 只在数据库异常 / 真实事务失败时发生（上层需要在事务外补写 CallbackLog）。
	ProcessWeChatCallback(ctx context.Context, input ProcessWeChatCallbackInput) (*ProcessWeChatCallbackResult, error)

	// WritePaymentCallbackLog 用于在事务之外补写一条 CallbackLog（验签失败或事务回滚场景）。
	WritePaymentCallbackLog(ctx context.Context, log PaymentCallbackLog) error

	ExistsPhoneInBrandUsers(ctx context.Context, phone string) (bool, error)
}
