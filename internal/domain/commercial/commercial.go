package commercial

import (
	"encoding/json"
	"time"
)

type SaaSPlanStatus string

const (
	SaaSPlanStatusActive   SaaSPlanStatus = "active"
	SaaSPlanStatusInactive SaaSPlanStatus = "inactive"
)

type BillingCycle string

const (
	BillingCycleMonthly BillingCycle = "monthly"
	BillingCycleYearly  BillingCycle = "yearly"
)

type PaymentChannel string

const (
	PaymentChannelWeChat PaymentChannel = "wechat"
	PaymentChannelAlipay PaymentChannel = "alipay"
)

type OrderSource string

const (
	OrderSourcePublicSignupFirstPurchase OrderSource = "public_signup_first_purchase"
	OrderSourceAdminManualCompensation   OrderSource = "admin_manual_compensation"
	OrderSourceBrandSelfServiceRenewal   OrderSource = "brand_self_service_renewal"
	OrderSourceBrandSelfServiceUpgrade   OrderSource = "brand_self_service_upgrade"
	OrderSourceBrandSelfServiceDowngrade OrderSource = "brand_self_service_downgrade"
)

type SaaSPlanOrderStatus string

const (
	SaaSPlanOrderStatusPendingPayment SaaSPlanOrderStatus = "pending_payment"
	SaaSPlanOrderStatusPaid           SaaSPlanOrderStatus = "paid"
	SaaSPlanOrderStatusClosed         SaaSPlanOrderStatus = "closed"
	SaaSPlanOrderStatusFailed         SaaSPlanOrderStatus = "failed"
	SaaSPlanOrderStatusRefunding      SaaSPlanOrderStatus = "refunding"
	SaaSPlanOrderStatusRefunded       SaaSPlanOrderStatus = "refunded"
	SaaSPlanOrderStatusException      SaaSPlanOrderStatus = "exception"
)

type BrandSubscriptionStatus string

const (
	BrandSubscriptionStatusActive      BrandSubscriptionStatus = "active"
	BrandSubscriptionStatusGracePeriod BrandSubscriptionStatus = "grace_period"
	BrandSubscriptionStatusRestricted  BrandSubscriptionStatus = "restricted"
	BrandSubscriptionStatusFrozen      BrandSubscriptionStatus = "frozen"
	BrandSubscriptionStatusExpired     BrandSubscriptionStatus = "expired"
	BrandSubscriptionStatusCancelled   BrandSubscriptionStatus = "cancelled"
)

type PaymentTransactionType string

const (
	PaymentTransactionTypePayment PaymentTransactionType = "payment"
	PaymentTransactionTypeRefund  PaymentTransactionType = "refund"
)

type PaymentTransactionStatus string

const (
	PaymentTransactionStatusPending   PaymentTransactionStatus = "pending"
	PaymentTransactionStatusSucceeded PaymentTransactionStatus = "succeeded"
	PaymentTransactionStatusFailed    PaymentTransactionStatus = "failed"
	PaymentTransactionStatusClosed    PaymentTransactionStatus = "closed"
	PaymentTransactionStatusRefunding PaymentTransactionStatus = "refunding"
	PaymentTransactionStatusRefunded  PaymentTransactionStatus = "refunded"
	PaymentTransactionStatusException PaymentTransactionStatus = "exception"
)

type PaymentCallbackLogStatus string

const (
	PaymentCallbackLogStatusReceived  PaymentCallbackLogStatus = "received"
	PaymentCallbackLogStatusProcessed PaymentCallbackLogStatus = "processed"
	PaymentCallbackLogStatusFailed    PaymentCallbackLogStatus = "failed"
	PaymentCallbackLogStatusIgnored   PaymentCallbackLogStatus = "ignored"
)

type SaaSPlan struct {
	ID                int64             `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	MonthlyPrice      string            `json:"monthly_price"`
	YearlyPrice       string            `json:"yearly_price"`
	YearlyDiscountPct *string           `json:"yearly_discount_pct,omitempty"`
	Currency          string            `json:"currency"`
	MaxLocations      int               `json:"max_locations"`
	MaxStaffSeats     int               `json:"max_staff_seats"`
	MaxLearners       int               `json:"max_learners"`
	Status            SaaSPlanStatus    `json:"status"`
	SortOrder         int               `json:"sort_order"`
	Features          []SaaSPlanFeature `json:"features,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type SaaSPlanFeature struct {
	ID          int64     `json:"id"`
	PlanID      int64     `json:"plan_id"`
	FeatureCode string    `json:"feature_code"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SaaSPlanOrder struct {
	ID                int64               `json:"id"`
	BrandID           int64               `json:"brand_id"`
	BrandUserID       *int64              `json:"brand_user_id,omitempty"`
	PlanID            int64               `json:"plan_id"`
	Source            OrderSource         `json:"source"`
	BillingCycle      BillingCycle        `json:"billing_cycle"`
	Amount            string              `json:"amount"`
	Currency          string              `json:"currency"`
	PaymentChannel    PaymentChannel      `json:"payment_channel"`
	Status            SaaSPlanOrderStatus `json:"status"`
	OutTradeNo        string              `json:"out_trade_no"`
	ThirdPartyTradeNo string              `json:"third_party_trade_no,omitempty"`
	WeChatCodeURL     string              `json:"wechat_code_url,omitempty"`
	WeChatPrepayID    string              `json:"wechat_prepay_id,omitempty"`
	PaymentExpiresAt  *time.Time          `json:"payment_expires_at,omitempty"`
	PaidAt            *time.Time          `json:"paid_at,omitempty"`
	ClosedAt          *time.Time          `json:"closed_at,omitempty"`
	FailureReason     string              `json:"failure_reason,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

type BrandSubscription struct {
	ID            int64                      `json:"id"`
	BrandID       int64                      `json:"brand_id"`
	PlanID        int64                      `json:"plan_id"`
	OrderID       *int64                     `json:"order_id,omitempty"`
	BillingCycle  BillingCycle               `json:"billing_cycle"`
	Status        BrandSubscriptionStatus    `json:"status"`
	StartsAt      time.Time                  `json:"starts_at"`
	ExpiresAt     time.Time                  `json:"expires_at"`
	GraceEndsAt   *time.Time                 `json:"grace_ends_at,omitempty"`
	MaxLocations  int                        `json:"max_locations"`
	MaxStaffSeats int                        `json:"max_staff_seats"`
	MaxLearners   int                        `json:"max_learners"`
	FrozenReason  string                     `json:"frozen_reason,omitempty"`
	Features      []BrandSubscriptionFeature `json:"features,omitempty"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

type BrandSubscriptionFeature struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	FeatureCode    string    `json:"feature_code"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type PaymentTransaction struct {
	ID                 int64                    `json:"id"`
	BrandID            *int64                   `json:"brand_id,omitempty"`
	OrderID            *int64                   `json:"order_id,omitempty"`
	PaymentChannel     PaymentChannel           `json:"payment_channel"`
	TransactionType    PaymentTransactionType   `json:"transaction_type"`
	Status             PaymentTransactionStatus `json:"status"`
	Amount             string                   `json:"amount"`
	Currency           string                   `json:"currency"`
	OutTradeNo         string                   `json:"out_trade_no"`
	ThirdPartyTradeNo  string                   `json:"third_party_trade_no,omitempty"`
	ProviderRequestID  string                   `json:"provider_request_id,omitempty"`
	CallbackReceivedAt *time.Time               `json:"callback_received_at,omitempty"`
	PaidAt             *time.Time               `json:"paid_at,omitempty"`
	FailureReason      string                   `json:"failure_reason,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

type PaymentCallbackLog struct {
	ID                int64                    `json:"id"`
	BrandID           *int64                   `json:"brand_id,omitempty"`
	OrderID           *int64                   `json:"order_id,omitempty"`
	TransactionID     *int64                   `json:"transaction_id,omitempty"`
	PaymentChannel    PaymentChannel           `json:"payment_channel"`
	OutTradeNo        string                   `json:"out_trade_no,omitempty"`
	ThirdPartyTradeNo string                   `json:"third_party_trade_no,omitempty"`
	CallbackRequestID string                   `json:"callback_request_id,omitempty"`
	Status            PaymentCallbackLogStatus `json:"status"`
	ProcessedAt       *time.Time               `json:"processed_at,omitempty"`
	ErrorMessage      string                   `json:"error_message,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
}

type OperationLog struct {
	ID         int64           `json:"id"`
	BrandID    *int64          `json:"brand_id,omitempty"`
	ActorType  string          `json:"actor_type"`
	ActorID    *int64          `json:"actor_id,omitempty"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type,omitempty"`
	TargetID   *int64          `json:"target_id,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}
