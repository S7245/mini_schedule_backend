// Package entitlement 权益应用服务（Batch 13b）。
//
// 编排：require(code) + 参数校验，落库委托 repo（额度账 / settle / 唯一冲突 / 引用在 repo）。
// 权益是品牌级（§21），无 data_scope；产品 CRUD+发放 gate entitlement.manage，对已发权益的
// 额度/状态干预 gate entitlement.adjust，只读 gate entitlement.view。
package entitlement

import (
	"context"
	"strings"

	domainent "github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker 是 service 需要的最小 Checker 面（仅 Require，无 data_scope）。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
}

// Service 权益应用服务。
type Service struct {
	repo    domainent.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限（兼容 bootstrap）。
func NewService(repo domainent.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

func validateMaxLen(field, v string, max int) error {
	if len([]rune(strings.TrimSpace(v))) > max {
		return apperr.NewAppError(apperr.ErrInvalidParam, field+"过长", 400)
	}
	return nil
}

// ---- 产品 ----

// ListProducts 产品列表。
func (s *Service) ListProducts(ctx context.Context, brandID, actorID int64, status, productType string, page, pageSize int) ([]*domainent.Product, int64, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.view"); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListProducts(ctx, domainent.ProductListFilter{
		BrandID: brandID, Status: status, ProductType: productType,
	}, (page-1)*pageSize, pageSize)
}

// GetProduct 产品详情。
func (s *Service) GetProduct(ctx context.Context, brandID, actorID, id int64) (*domainent.Product, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.view"); err != nil {
		return nil, err
	}
	return s.repo.GetProduct(ctx, brandID, id)
}

// CreateProduct 创建产品。
func (s *Service) CreateProduct(ctx context.Context, in domainent.CreateProductInput) (*domainent.Product, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "entitlement.manage"); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "产品名称不能为空", 400)
	}
	if err := validateMaxLen("产品名称", name, 100); err != nil {
		return nil, err
	}
	if err := validateMaxLen("产品描述", in.Description, 1000); err != nil {
		return nil, err
	}
	if !domainent.IsValidProductType(in.ProductType) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的产品类型", 400)
	}
	if domainent.IsCountBased(domainent.ProductType(in.ProductType)) && in.TotalCredits <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "次数/课时包必须设置大于 0 的次数", 400)
	}
	if in.ValidityDays <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "有效期天数必须大于 0", 400)
	}
	if !domainent.IsValidScope(in.LocationScope) || !domainent.IsValidScope(in.CourseScope) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的适用范围", 400)
	}
	in.Name = name
	return s.repo.CreateProduct(ctx, in)
}

// UpdateProduct 编辑产品（白名单，product_type 不可改）。
func (s *Service) UpdateProduct(ctx context.Context, brandID, actorID, id int64, in domainent.UpdateProductInput) (*domainent.Product, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.manage"); err != nil {
		return nil, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "产品名称不能为空", 400)
		}
		if err := validateMaxLen("产品名称", v, 100); err != nil {
			return nil, err
		}
		in.Name = &v
	}
	if in.Description != nil {
		if err := validateMaxLen("产品描述", *in.Description, 1000); err != nil {
			return nil, err
		}
	}
	if in.TotalCredits != nil && *in.TotalCredits <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "次数必须大于 0", 400)
	}
	if in.ValidityDays != nil && *in.ValidityDays <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "有效期天数必须大于 0", 400)
	}
	if in.LocationScope != nil && !domainent.IsValidScope(*in.LocationScope) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的门店适用范围", 400)
	}
	if in.CourseScope != nil && !domainent.IsValidScope(*in.CourseScope) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的课程适用范围", 400)
	}
	return s.repo.UpdateProduct(ctx, brandID, actorID, id, in)
}

// UpdateProductStatus 启停产品。
func (s *Service) UpdateProductStatus(ctx context.Context, brandID, actorID, id int64, status string) (*domainent.Product, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.manage"); err != nil {
		return nil, err
	}
	if !domainent.IsValidProductStatus(status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的产品状态", 400)
	}
	return s.repo.UpdateProductStatus(ctx, brandID, actorID, id, status)
}

// ---- 学员权益 ----

// ListByLearner 学员权益列表（含 settle）。
func (s *Service) ListByLearner(ctx context.Context, brandID, actorID, learnerID int64) ([]*domainent.Entitlement, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.view"); err != nil {
		return nil, err
	}
	return s.repo.ListEntitlementsByLearner(ctx, brandID, learnerID)
}

// ListTransactions 权益流水。
func (s *Service) ListTransactions(ctx context.Context, brandID, actorID, entitlementID int64) ([]*domainent.Transaction, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.view"); err != nil {
		return nil, err
	}
	return s.repo.ListTransactions(ctx, brandID, entitlementID)
}

// Grant 发放权益给学员。
func (s *Service) Grant(ctx context.Context, in domainent.GrantInput) (*domainent.Entitlement, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "entitlement.manage"); err != nil {
		return nil, err
	}
	if in.ProductID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "请选择权益产品", 400)
	}
	if in.LearnerID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "学员无效", 400)
	}
	if err := validateMaxLen("备注", in.Remark, 1000); err != nil {
		return nil, err
	}
	return s.repo.Grant(ctx, in)
}

// Adjust 手动额度调整（必填原因，delta 非 0）。
func (s *Service) Adjust(ctx context.Context, in domainent.AdjustInput) (*domainent.Entitlement, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "entitlement.adjust"); err != nil {
		return nil, err
	}
	if in.Delta == 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "调整数量不能为 0", 400)
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "请填写调整原因", 400)
	}
	if err := validateMaxLen("原因", in.Reason, 1000); err != nil {
		return nil, err
	}
	return s.repo.Adjust(ctx, in)
}

// SetStatus 冻结 / 作废 / 恢复学员权益。
func (s *Service) SetStatus(ctx context.Context, brandID, actorID, id int64, status, reason string) (*domainent.Entitlement, error) {
	if err := s.require(ctx, brandID, actorID, "entitlement.adjust"); err != nil {
		return nil, err
	}
	switch status {
	case string(domainent.StatusActive), string(domainent.StatusFrozen), string(domainent.StatusCancelled):
	default:
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的权益状态（仅支持 冻结/作废/恢复）", 400)
	}
	if err := validateMaxLen("原因", reason, 1000); err != nil {
		return nil, err
	}
	return s.repo.SetEntitlementStatus(ctx, brandID, actorID, id, status, reason)
}
