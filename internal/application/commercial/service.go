package commercial

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/payment"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo          commercial.Repository
	cfg           *config.Config
	redis         *redis.Client
	wechatAdapter *payment.WeChatPaymentAdapter
}

func NewService(
	repo commercial.Repository,
	cfg *config.Config,
	redisClient *redis.Client,
	wechatAdapter *payment.WeChatPaymentAdapter,
) *Service {
	return &Service{
		repo:          repo,
		cfg:           cfg,
		redis:         redisClient,
		wechatAdapter: wechatAdapter,
	}
}

func (s *Service) CreateSaaSPlan(ctx context.Context, input commercial.CreateSaaSPlanInput) (*commercial.SaaSPlan, error) {
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	return s.repo.CreateSaaSPlan(ctx, input)
}

func (s *Service) GetSaaSPlan(ctx context.Context, id int64) (*commercial.SaaSPlan, error) {
	return s.repo.GetSaaSPlan(ctx, id)
}

func (s *Service) ListSaaSPlans(ctx context.Context, page, pageSize int, includeInactive bool) ([]*commercial.SaaSPlan, int64, error) {
	offset, limit := s.pagination(page, pageSize)
	return s.repo.ListSaaSPlans(ctx, offset, limit, includeInactive)
}

func (s *Service) ListPublicSaaSPlans(ctx context.Context) ([]*commercial.SaaSPlan, error) {
	return s.repo.ListPublicSaaSPlans(ctx)
}

func (s *Service) CreatePublicSignupOrder(ctx context.Context, input commercial.CreatePublicSignupOrderInput) (*commercial.PublicSignupOrderResult, error) {
	input.Phone = strings.TrimSpace(input.Phone)
	input.SMSCode = strings.TrimSpace(input.SMSCode)
	input.BrandName = strings.TrimSpace(input.BrandName)
	input.ContactName = strings.TrimSpace(input.ContactName)
	input.ContactEmail = strings.TrimSpace(input.ContactEmail)
	input.IndustryType = strings.TrimSpace(input.IndustryType)

	if input.Phone == "" {
		return nil, apperr.ErrBadRequest("手机号不能为空")
	}
	if input.SMSCode == "" {
		return nil, apperr.ErrBadRequest("短信验证码不能为空")
	}
	if input.Password == "" {
		return nil, apperr.ErrBadRequest("密码不能为空")
	}
	if len(input.Password) < 6 || len(input.Password) > 64 {
		return nil, apperr.ErrBadRequest("密码长度必须为 6-64 位")
	}
	if input.BrandName == "" {
		return nil, apperr.ErrBadRequest("品牌名称不能为空")
	}
	if input.ContactName == "" {
		return nil, apperr.ErrBadRequest("联系人姓名不能为空")
	}
	if input.PlanID <= 0 {
		return nil, apperr.ErrBadRequest("请选择 SaaS 套餐")
	}
	if input.BillingCycle == "" {
		input.BillingCycle = commercial.BillingCycleMonthly
	}
	switch input.BillingCycle {
	case commercial.BillingCycleMonthly, commercial.BillingCycleYearly:
	default:
		return nil, apperr.ErrBadRequest("计费周期无效")
	}
	if input.PaymentChannel == "" {
		input.PaymentChannel = commercial.PaymentChannelWeChat
	}
	switch input.PaymentChannel {
	case commercial.PaymentChannelWeChat, commercial.PaymentChannelAlipay:
	default:
		return nil, apperr.ErrBadRequest("支付通道无效")
	}
	if err := s.verifySignupSMSCode(ctx, input.Phone, input.SMSCode); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.ErrInternalF("密码加密失败", err)
	}
	outTradeNo, err := generateOutTradeNo()
	if err != nil {
		return nil, apperr.ErrInternalF("生成订单号失败", err)
	}

	return s.repo.CreatePublicSignupOrder(ctx, commercial.CreatePublicSignupOrderRecordInput{
		Phone:          input.Phone,
		PasswordHash:   string(hashedPassword),
		BrandName:      input.BrandName,
		LogoURL:        input.LogoURL,
		ContactName:    input.ContactName,
		ContactEmail:   input.ContactEmail,
		IndustryType:   input.IndustryType,
		PlanID:         input.PlanID,
		BillingCycle:   input.BillingCycle,
		PaymentChannel: input.PaymentChannel,
		OutTradeNo:     outTradeNo,
	})
}

func (s *Service) UpdateSaaSPlan(ctx context.Context, id int64, input commercial.UpdateSaaSPlanInput) (*commercial.SaaSPlan, error) {
	return s.repo.UpdateSaaSPlan(ctx, id, input)
}

func (s *Service) UpdateSaaSPlanStatus(ctx context.Context, id int64, status commercial.SaaSPlanStatus) error {
	return s.repo.UpdateSaaSPlanStatus(ctx, id, status)
}

func (s *Service) ListSaaSPlanOrders(ctx context.Context, page, pageSize int, filter commercial.ListSaaSPlanOrdersFilter) ([]*commercial.SaaSPlanOrder, int64, error) {
	offset, limit := s.pagination(page, pageSize)
	return s.repo.ListSaaSPlanOrders(ctx, offset, limit, filter)
}

func (s *Service) ListBrandSubscriptions(ctx context.Context, page, pageSize int, filter commercial.ListBrandSubscriptionsFilter) ([]*commercial.BrandSubscription, int64, error) {
	offset, limit := s.pagination(page, pageSize)
	return s.repo.ListBrandSubscriptions(ctx, offset, limit, filter)
}

func (s *Service) ManualRenewBrandSubscription(ctx context.Context, id int64, input commercial.ManualRenewBrandSubscriptionInput) (*commercial.BrandSubscription, error) {
	if input.ExtendMonths <= 0 && input.ExtendDays <= 0 {
		return nil, apperr.ErrBadRequest("续期月份或天数必须大于 0")
	}
	if input.ExtendMonths < 0 || input.ExtendDays < 0 {
		return nil, apperr.ErrBadRequest("续期月份或天数不能为负数")
	}
	if input.Reason == "" {
		return nil, apperr.ErrBadRequest("请填写操作原因")
	}
	return s.repo.ManualRenewBrandSubscription(ctx, id, input)
}

func (s *Service) UpdateBrandSubscriptionLimits(ctx context.Context, id int64, input commercial.UpdateBrandSubscriptionLimitsInput) (*commercial.BrandSubscription, error) {
	if input.MaxLocations == nil && input.MaxStaffSeats == nil && input.MaxLearners == nil && input.Features == nil {
		return nil, apperr.ErrBadRequest("请至少填写一个要调整的额度或功能")
	}
	if input.MaxLocations != nil && *input.MaxLocations <= 0 {
		return nil, apperr.ErrBadRequest("Location 额度必须大于 0")
	}
	if input.MaxStaffSeats != nil && *input.MaxStaffSeats <= 0 {
		return nil, apperr.ErrBadRequest("员工席位额度必须大于 0")
	}
	if input.MaxLearners != nil && *input.MaxLearners <= 0 {
		return nil, apperr.ErrBadRequest("学员额度必须大于 0")
	}
	if input.Reason == "" {
		return nil, apperr.ErrBadRequest("请填写操作原因")
	}
	return s.repo.UpdateBrandSubscriptionLimits(ctx, id, input)
}

func (s *Service) UpdateBrandSubscriptionStatus(ctx context.Context, id int64, input commercial.UpdateBrandSubscriptionStatusInput) (*commercial.BrandSubscription, error) {
	switch input.Status {
	case commercial.BrandSubscriptionStatusActive,
		commercial.BrandSubscriptionStatusGracePeriod,
		commercial.BrandSubscriptionStatusRestricted,
		commercial.BrandSubscriptionStatusFrozen,
		commercial.BrandSubscriptionStatusExpired,
		commercial.BrandSubscriptionStatusCancelled:
	default:
		return nil, apperr.ErrBadRequest("订阅状态无效")
	}
	if input.Status == commercial.BrandSubscriptionStatusFrozen && input.FrozenReason == "" {
		return nil, apperr.ErrBadRequest("冻结订阅时必须填写冻结原因")
	}
	if input.Reason == "" {
		return nil, apperr.ErrBadRequest("请填写操作原因")
	}
	return s.repo.UpdateBrandSubscriptionStatus(ctx, id, input)
}

func (s *Service) ListPaymentTransactions(ctx context.Context, page, pageSize int, filter commercial.ListPaymentTransactionsFilter) ([]*commercial.PaymentTransaction, int64, error) {
	offset, limit := s.pagination(page, pageSize)
	return s.repo.ListPaymentTransactions(ctx, offset, limit, filter)
}

func (s *Service) ListPaymentCallbackLogs(ctx context.Context, page, pageSize int, status commercial.PaymentCallbackLogStatus) ([]*commercial.PaymentCallbackLog, int64, error) {
	offset, limit := s.pagination(page, pageSize)
	return s.repo.ListPaymentCallbackLogs(ctx, offset, limit, status)
}

func (s *Service) GetPlatformSummary(ctx context.Context) (*commercial.PlatformSummary, error) {
	return s.repo.GetPlatformSummary(ctx)
}

func (s *Service) ListOperationLogs(ctx context.Context, page, pageSize int, filter commercial.ListOperationLogsFilter) ([]*commercial.OperationLog, int64, error) {
	offset, limit := s.pagination(page, pageSize)
	return s.repo.ListOperationLogs(ctx, offset, limit, filter)
}

func (s *Service) RequestSignupSMSCode(ctx context.Context, phone, ip string) error {
	if phone == "" {
		return apperr.ErrBadRequest("手机号不能为空")
	}

	// Rate limit: same phone, max 5 per day
	phoneKey := fmt.Sprintf("sms:rate:phone:%s", phone)
	phoneCount, err := s.redis.Incr(ctx, phoneKey).Result()
	if err != nil {
		return apperr.ErrInternalF("限流检查失败", err)
	}
	if phoneCount == 1 {
		s.redis.Expire(ctx, phoneKey, 24*time.Hour)
	}
	if phoneCount > 5 {
		return apperr.NewAppError(apperr.ErrTooManyRequests, "该号码今日发送次数已达上限", 429)
	}

	// Rate limit: same IP, max 10 per hour
	if ip != "" {
		ipKey := fmt.Sprintf("sms:rate:ip:%s", ip)
		ipCount, err := s.redis.Incr(ctx, ipKey).Result()
		if err != nil {
			return apperr.ErrInternalF("限流检查失败", err)
		}
		if ipCount == 1 {
			s.redis.Expire(ctx, ipKey, time.Hour)
		}
		if ipCount > 10 {
			return apperr.NewAppError(apperr.ErrTooManyRequests, "请求过于频繁，请稍后再试", 429)
		}
	}

	if s.cfg.App.Env == "production" && !s.cfg.SMS.AllowMock && s.cfg.SMS.Provider == "" {
		return apperr.ErrInternal("短信服务未配置")
	}
	if s.cfg.App.Env == "production" && !s.cfg.SMS.AllowMock && s.cfg.SMS.Provider != "mock" {
		return apperr.ErrInternal("短信服务 Provider 尚未接入")
	}
	// The first implementation keeps the public API contract stable and uses
	// mock SMS in development. A real provider can plug in behind this method.
	return nil
}

var (
	rePhone      = regexp.MustCompile(`^1[3-9]\d{9}$`)
	reHasLetter  = regexp.MustCompile(`[a-zA-Z]`)
	reHasDigit   = regexp.MustCompile(`[0-9]`)
)

func (s *Service) PreValidateSignup(ctx context.Context, phone, smsCode, password string) error {
	phone = strings.TrimSpace(phone)
	smsCode = strings.TrimSpace(smsCode)

	if !rePhone.MatchString(phone) {
		return apperr.ErrBadRequest("手机号格式不正确")
	}
	if len(password) < 8 || !reHasLetter.MatchString(password) || !reHasDigit.MatchString(password) {
		return apperr.ErrBadRequest("密码至少 8 位，且必须同时包含字母和数字")
	}
	if err := s.verifySignupSMSCode(ctx, phone, smsCode); err != nil {
		return err
	}
	exists, err := s.repo.ExistsPhoneInBrandUsers(ctx, phone)
	if err != nil {
		return err
	}
	if exists {
		return apperr.ErrBadRequest("该手机号已注册")
	}
	return nil
}

func (s *Service) verifySignupSMSCode(ctx context.Context, phone, code string) error {
	_ = ctx
	_ = phone
	mockCode := s.cfg.SMS.MockCode
	if mockCode == "" {
		mockCode = "123456"
	}

	if s.cfg.App.Env == "production" && !s.cfg.SMS.AllowMock && s.cfg.SMS.Provider == "" {
		return apperr.ErrInternal("短信服务未配置")
	}
	if s.cfg.SMS.AllowMock || s.cfg.SMS.Provider == "" || s.cfg.SMS.Provider == "mock" || s.cfg.App.Env != "production" {
		if code != mockCode {
			return apperr.ErrBadRequest("短信验证码错误")
		}
		return nil
	}

	return apperr.ErrInternal("短信验证码验证服务未接入")
}

func generateOutTradeNo() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("MS%s%s", time.Now().UTC().Format("20060102150405"), hex.EncodeToString(b[:])), nil
}

type NativePayResult struct {
	CodeURL   string    `json:"code_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type OrderPaymentStatus struct {
	Status string     `json:"status"`
	PaidAt *time.Time `json:"paid_at"`
}

func (s *Service) CreateWeChatNativePay(ctx context.Context, orderID int64) (*NativePayResult, error) {
	expiresAt := time.Now().Add(2 * time.Hour)

	// 未配置微信支付（本地开发）：使用 mock code_url，但仍验证订单存在
	if s.cfg.Payment.WeChat.AppID == "" || s.cfg.Payment.WeChat.MchID == "" {
		mockCodeURL := fmt.Sprintf("weixin://wxpay/bizpayurl?mock=1&order_id=%d", orderID)
		if err := s.repo.CreateWeChatNativePayOrder(ctx, orderID, mockCodeURL, "mock_prepay_id", expiresAt); err != nil {
			return nil, err
		}
		return &NativePayResult{CodeURL: mockCodeURL, ExpiresAt: expiresAt}, nil
	}

	// 已配置微信支付：调用真实 API（TODO: 实现 RSA-SHA256 签名）
	codeURL := fmt.Sprintf("weixin://wxpay/bizpayurl?order_id=%d", orderID)
	prepayID := ""

	if err := s.repo.CreateWeChatNativePayOrder(ctx, orderID, codeURL, prepayID, expiresAt); err != nil {
		return nil, err
	}
	return &NativePayResult{CodeURL: codeURL, ExpiresAt: expiresAt}, nil
}

func (s *Service) GetOrderPaymentStatus(ctx context.Context, orderID int64) (*OrderPaymentStatus, error) {
	// 未配置微信支付（本地开发）：返回 mock pending 状态
	if s.cfg.Payment.WeChat.AppID == "" || s.cfg.Payment.WeChat.MchID == "" {
		return &OrderPaymentStatus{Status: "pending_payment", PaidAt: nil}, nil
	}

	result, err := s.repo.GetSaaSPlanOrderStatus(ctx, orderID)
	if err != nil {
		return nil, err
	}
	return &OrderPaymentStatus{Status: result.Status, PaidAt: result.PaidAt}, nil
}

func (s *Service) pagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = s.cfg.Pagination.DefaultPageSize
	}
	if pageSize > s.cfg.Pagination.MaxPageSize {
		pageSize = s.cfg.Pagination.MaxPageSize
	}
	return (page - 1) * pageSize, pageSize
}
