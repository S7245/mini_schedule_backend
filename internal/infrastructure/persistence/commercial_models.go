package persistence

import (
	"time"

	"gorm.io/gorm"
)

type TimestampModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SaaSPlanModel struct {
	TimestampModel
	Name              string                 `gorm:"size:100;not null" json:"name"`
	Description       string                 `gorm:"size:1000" json:"description"`
	MonthlyPrice      string                 `gorm:"type:numeric(12,2);not null;default:0" json:"monthly_price"`
	YearlyPrice       string                 `gorm:"type:numeric(12,2);not null;default:0" json:"yearly_price"`
	YearlyDiscountPct *string                `gorm:"type:numeric(5,2)" json:"yearly_discount_pct"`
	Currency          string                 `gorm:"size:3;not null;default:CNY" json:"currency"`
	MaxLocations      int                    `gorm:"not null" json:"max_locations"`
	MaxStaffSeats     int                    `gorm:"not null" json:"max_staff_seats"`
	MaxLearners       int                    `gorm:"not null" json:"max_learners"`
	Status            string                 `gorm:"size:20;not null;default:active" json:"status"`
	SortOrder         int                    `gorm:"not null;default:0" json:"sort_order"`
	Features          []SaaSPlanFeatureModel `gorm:"foreignKey:PlanID" json:"features,omitempty"`
}

func (SaaSPlanModel) TableName() string { return "saas_plans" }

type SaaSPlanFeatureModel struct {
	TimestampModel
	PlanID      int64  `gorm:"not null;index" json:"plan_id"`
	FeatureCode string `gorm:"size:80;not null" json:"feature_code"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
}

func (SaaSPlanFeatureModel) TableName() string { return "saas_plan_features" }

type SaaSPlanOrderModel struct {
	TimestampModel
	BrandID                int64      `gorm:"not null;index" json:"brand_id"`
	BrandUserID            *int64     `gorm:"index" json:"brand_user_id"`
	PlanID                 int64      `gorm:"not null;index" json:"plan_id"`
	Source                 string     `gorm:"size:40;not null;default:public_signup_first_purchase" json:"source"`
	BillingCycle           string     `gorm:"size:20;not null" json:"billing_cycle"`
	Amount                 string     `gorm:"type:numeric(12,2);not null" json:"amount"`
	Currency               string     `gorm:"size:3;not null;default:CNY" json:"currency"`
	PaymentChannel         string     `gorm:"size:20;not null" json:"payment_channel"`
	Status                 string     `gorm:"size:30;not null;default:pending_payment" json:"status"`
	OutTradeNo             string     `gorm:"size:100;not null;uniqueIndex" json:"out_trade_no"`
	ThirdPartyTradeNo      string     `gorm:"size:100" json:"third_party_trade_no"`
	WeChatCodeURL          string     `gorm:"column:wechat_code_url;size:500" json:"wechat_code_url"`
	WeChatPrepayID         string     `gorm:"column:wechat_prepay_id;size:100" json:"wechat_prepay_id"`
	PaymentRequestPayload  []byte     `gorm:"type:jsonb" json:"-"`
	PaymentResponsePayload []byte     `gorm:"type:jsonb" json:"-"`
	PaymentExpiresAt       *time.Time `json:"payment_expires_at"`
	PaidAt                 *time.Time `json:"paid_at"`
	ClosedAt               *time.Time `json:"closed_at"`
	FailureReason          string     `gorm:"size:500" json:"failure_reason"`
}

func (SaaSPlanOrderModel) TableName() string { return "saas_plan_orders" }

type BrandSubscriptionModel struct {
	TimestampModel
	BrandID       int64                           `gorm:"not null;index" json:"brand_id"`
	PlanID        int64                           `gorm:"not null;index" json:"plan_id"`
	OrderID       *int64                          `gorm:"index" json:"order_id"`
	BillingCycle  string                          `gorm:"size:20;not null" json:"billing_cycle"`
	Status        string                          `gorm:"size:30;not null;default:active" json:"status"`
	StartsAt      time.Time                       `gorm:"not null" json:"starts_at"`
	ExpiresAt     time.Time                       `gorm:"not null" json:"expires_at"`
	GraceEndsAt   *time.Time                      `json:"grace_ends_at"`
	MaxLocations  int                             `gorm:"not null" json:"max_locations"`
	MaxStaffSeats int                             `gorm:"not null" json:"max_staff_seats"`
	MaxLearners   int                             `gorm:"not null" json:"max_learners"`
	FrozenReason  string                          `gorm:"size:500" json:"frozen_reason"`
	Features      []BrandSubscriptionFeatureModel `gorm:"foreignKey:SubscriptionID" json:"features,omitempty"`
}

func (BrandSubscriptionModel) TableName() string { return "brand_subscriptions" }

type BrandSubscriptionFeatureModel struct {
	TimestampModel
	SubscriptionID int64  `gorm:"not null;index" json:"subscription_id"`
	FeatureCode    string `gorm:"size:80;not null" json:"feature_code"`
	Enabled        bool   `gorm:"not null;default:true" json:"enabled"`
}

func (BrandSubscriptionFeatureModel) TableName() string {
	return "brand_subscription_features"
}

type PaymentTransactionModel struct {
	TimestampModel
	BrandID            *int64     `gorm:"index" json:"brand_id"`
	OrderID            *int64     `gorm:"index" json:"order_id"`
	PaymentChannel     string     `gorm:"size:20;not null" json:"payment_channel"`
	TransactionType    string     `gorm:"size:20;not null;default:payment" json:"transaction_type"`
	Status             string     `gorm:"size:30;not null;default:pending" json:"status"`
	Amount             string     `gorm:"type:numeric(12,2);not null" json:"amount"`
	Currency           string     `gorm:"size:3;not null;default:CNY" json:"currency"`
	OutTradeNo         string     `gorm:"size:100;not null" json:"out_trade_no"`
	ThirdPartyTradeNo  string     `gorm:"size:100" json:"third_party_trade_no"`
	ProviderRequestID  string     `gorm:"size:100" json:"provider_request_id"`
	RequestPayload     []byte     `gorm:"type:jsonb" json:"-"`
	ResponsePayload    []byte     `gorm:"type:jsonb" json:"-"`
	CallbackPayload    []byte     `gorm:"type:jsonb" json:"-"`
	CallbackReceivedAt *time.Time `json:"callback_received_at"`
	PaidAt             *time.Time `json:"paid_at"`
	FailureReason      string     `gorm:"size:500" json:"failure_reason"`
}

func (PaymentTransactionModel) TableName() string { return "payment_transactions" }

// BeforeCreate 兜底所有 JSONB 列，防止 nil 切片 → SQL NULL → 23502。
// payment_transactions 的 request/response/callback_payload 在迁移里允许 NULL，
// 当前未触发问题，但保留 hook 与 BatchX 习惯一致，避免后续加 NOT NULL 时漏改。
func (m *PaymentTransactionModel) BeforeCreate(*gorm.DB) error {
	if len(m.RequestPayload) == 0 {
		m.RequestPayload = []byte("{}")
	}
	if len(m.ResponsePayload) == 0 {
		m.ResponsePayload = []byte("{}")
	}
	if len(m.CallbackPayload) == 0 {
		m.CallbackPayload = []byte("{}")
	}
	return nil
}

// BeforeCreate 兜底 saas_plan_orders 的 JSONB 列。当前列允许 NULL，但与 PaymentTransaction 保持一致策略。
func (m *SaaSPlanOrderModel) BeforeCreate(*gorm.DB) error {
	if len(m.PaymentRequestPayload) == 0 {
		m.PaymentRequestPayload = []byte("{}")
	}
	if len(m.PaymentResponsePayload) == 0 {
		m.PaymentResponsePayload = []byte("{}")
	}
	return nil
}

type PaymentCallbackLogModel struct {
	ID                int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt         time.Time  `json:"created_at"`
	BrandID           *int64     `gorm:"index" json:"brand_id"`
	OrderID           *int64     `gorm:"index" json:"order_id"`
	TransactionID     *int64     `gorm:"index" json:"transaction_id"`
	PaymentChannel    string     `gorm:"size:20;not null" json:"payment_channel"`
	OutTradeNo        string     `gorm:"size:100" json:"out_trade_no"`
	ThirdPartyTradeNo string     `gorm:"size:100" json:"third_party_trade_no"`
	CallbackRequestID string     `gorm:"size:100" json:"callback_request_id"`
	Status            string     `gorm:"size:30;not null;default:received" json:"status"`
	Headers           []byte     `gorm:"type:jsonb" json:"-"`
	RawBody           string     `json:"-"`
	Payload           []byte     `gorm:"type:jsonb" json:"-"`
	ProcessedAt       *time.Time `json:"processed_at"`
	ErrorMessage      string     `gorm:"size:1000" json:"error_message"`
}

func (PaymentCallbackLogModel) TableName() string { return "payment_callback_logs" }

func (m *PaymentCallbackLogModel) BeforeCreate(*gorm.DB) error {
	if len(m.Headers) == 0 {
		m.Headers = []byte("{}")
	}
	if len(m.Payload) == 0 {
		m.Payload = []byte("{}")
	}
	return nil
}

type OperationLogModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	BrandID    *int64    `gorm:"index" json:"brand_id"`
	ActorType  string    `gorm:"size:30;not null" json:"actor_type"`
	ActorID    *int64    `json:"actor_id"`
	Action     string    `gorm:"size:100;not null" json:"action"`
	TargetType string    `gorm:"size:80" json:"target_type"`
	TargetID   *int64    `json:"target_id"`
	Reason     string    `gorm:"size:1000" json:"reason"`
	Metadata   []byte    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
}

func (OperationLogModel) TableName() string { return "operation_logs" }

// BeforeCreate 兜底 operation_logs.metadata JSONB NOT NULL 列。
// 当前所有调用点都显式赋值，但补 hook 防止后续遗漏 → 23502。
func (m *OperationLogModel) BeforeCreate(*gorm.DB) error {
	if len(m.Metadata) == 0 {
		m.Metadata = []byte("{}")
	}
	return nil
}
