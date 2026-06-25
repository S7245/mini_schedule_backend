// Package learnerbooking C 端学员自助预约应用服务（Batch 14a）。
//
// 与 brand 侧 booking.Service 的关键区别：**无 RBAC**（学员不是 brand_user，无品牌权限码）。
// 鉴权 = 所有权——所有"我的"查询/操作按登录 token 的 brand_learner_profile_id 收口（List 强制
// 过滤本 profile；Cancel 在 repo tx 内校所有权）。复用 RBAC-free 的 booking.Repository /
// classsession.Repository（ScopeLocationIDs 恒 nil，学员无 data_scope），不复用 brand Service。
package learnerbooking

import (
	"context"
	"time"

	domainbooking "github.com/zkw/mini-schedule/backend/internal/domain/booking"
	domainsession "github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// Service 学员自助预约应用服务。
type Service struct {
	bookingRepo domainbooking.Repository
	sessionRepo domainsession.Repository
}

// NewService 创建 Service。
func NewService(bookingRepo domainbooking.Repository, sessionRepo domainsession.Repository) *Service {
	return &Service{bookingRepo: bookingRepo, sessionRepo: sessionRepo}
}

// requireProfile 校验登录态带 profile（token 无 profile_id → 桥接前旧 token，需重新登录）。
func requireProfile(profileID int64) error {
	if profileID <= 0 {
		return apperr.ErrUnauthorizedF("请重新登录")
	}
	return nil
}

func clampPage(page, pageSize int) (offset, limit int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return (page - 1) * pageSize, pageSize
}

// ListSessions C 端课程表：brand + scheduled + 未来场次（soonest first）。无 data_scope，只读。
func (s *Service) ListSessions(ctx context.Context, brandID int64, page, pageSize int) ([]*domainsession.Session, int64, error) {
	now := time.Now().UTC()
	offset, limit := clampPage(page, pageSize)
	return s.sessionRepo.List(ctx, domainsession.ListFilter{
		BrandID: brandID,
		Status:  string(domainsession.StatusScheduled),
		From:    &now,
	}, offset, limit)
}

// GetSession 场次详情（brand 范围只读；不存在/越 brand → SESSION_NOT_FOUND）。
func (s *Service) GetSession(ctx context.Context, brandID, id int64) (*domainsession.Session, error) {
	return s.sessionRepo.GetByID(ctx, brandID, id)
}

// Book 自助预约（auto / source=learner_self_service / assisted_by NULL，repo 内含 §22.1 重叠校验）。
func (s *Service) Book(ctx context.Context, brandID, profileID, sessionID int64) (*domainbooking.Booking, error) {
	if err := requireProfile(profileID); err != nil {
		return nil, err
	}
	if sessionID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "场次不能为空", 400)
	}
	return s.bookingRepo.CreateByLearner(ctx, domainbooking.LearnerCreateInput{
		BrandID:               brandID,
		ClassSessionID:        sessionID,
		BrandLearnerProfileID: profileID,
	})
}

// ListMyBookings 我的预约（**强制**按 token profile 过滤，不接受前端传 learner 参数）。status 可选。
func (s *Service) ListMyBookings(ctx context.Context, brandID, profileID int64, status string, page, pageSize int) ([]*domainbooking.Booking, int64, error) {
	if err := requireProfile(profileID); err != nil {
		return nil, 0, err
	}
	offset, limit := clampPage(page, pageSize)
	return s.bookingRepo.List(ctx, domainbooking.ListFilter{
		BrandID:               brandID,
		BrandLearnerProfileID: profileID,
		Status:                status,
	}, offset, limit)
}

// CancelMyBooking 自助取消（所有权在 repo tx 内校验，越权 BOOKING_NOT_FOUND）。
func (s *Service) CancelMyBooking(ctx context.Context, brandID, profileID, id int64, reason string) (*domainbooking.Booking, error) {
	if err := requireProfile(profileID); err != nil {
		return nil, err
	}
	return s.bookingRepo.CancelByLearner(ctx, brandID, profileID, id, reason)
}

// UsableEntitlements 预约前预览本人对某场次的可用权益（§5.7 序，仅展示——学员第一版不自选权益）。
func (s *Service) UsableEntitlements(ctx context.Context, brandID, profileID, sessionID int64) ([]*domainbooking.UsableEntitlement, error) {
	if err := requireProfile(profileID); err != nil {
		return nil, err
	}
	return s.bookingRepo.UsableEntitlements(ctx, brandID, sessionID, profileID, nil)
}
