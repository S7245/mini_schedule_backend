package commercial

import (
	"context"

	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
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
	if s.cfg.App.Env == "production" && s.cfg.SMS.Provider == "" {
		return apperr.ErrInternal("短信服务未配置")
	}
	// The first implementation keeps the public API contract stable and uses
	// mock SMS in development. A real provider can plug in behind this method.
	return nil
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
