package commercial

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo commercial.Repository
	cfg  *config.Config
}

func NewService(repo commercial.Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
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

func (s *Service) RequestSignupSMSCode(ctx context.Context, phone string) error {
	_ = ctx
	if phone == "" {
		return apperr.ErrBadRequest("手机号不能为空")
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
