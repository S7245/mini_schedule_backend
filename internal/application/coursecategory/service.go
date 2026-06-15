// Package coursecategory 课程分类应用服务（Batch 11）。
package coursecategory

import (
	"context"
	"strings"

	domaincat "github.com/zkw/mini-schedule/backend/internal/domain/coursecategory"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker 是 service 需要的最小 Checker 面。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
}

// Service 课程分类应用服务。
type Service struct {
	repo    domaincat.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限校验（兼容 bootstrap）。
func NewService(repo domaincat.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// CreateInput 创建入参。
type CreateInput struct {
	BrandID           int64
	ActorID           int64
	Name              string
	Color             string
	Icon              string
	SortOrder         int
	ShowInMiniProgram bool
}

// UpdateInput 更新入参（白名单）。
type UpdateInput struct {
	Name              *string
	Color             *string
	Icon              *string
	SortOrder         *int
	ShowInMiniProgram *bool
	Status            *string
}

// List 列表（状态筛选）。
func (s *Service) List(ctx context.Context, brandID, actorID int64, status string) ([]*domaincat.Category, error) {
	if err := s.require(ctx, brandID, actorID, "course_category.view"); err != nil {
		return nil, err
	}
	if status == "all" {
		status = ""
	}
	return s.repo.List(ctx, domaincat.ListFilter{BrandID: brandID, Status: status})
}

// Create 创建分类。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domaincat.Category, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "course_category.create"); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "分类名称不能为空", 400)
	}
	if len([]rune(name)) > 100 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "分类名称过长", 400)
	}
	return s.repo.Create(ctx, domaincat.CreateInput{
		BrandID:           in.BrandID,
		ActorID:           in.ActorID,
		Name:              name,
		Color:             in.Color,
		Icon:              in.Icon,
		SortOrder:         in.SortOrder,
		ShowInMiniProgram: in.ShowInMiniProgram,
	})
}

// Update 编辑分类（含状态切换）。
func (s *Service) Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*domaincat.Category, error) {
	if err := s.require(ctx, brandID, actorID, "course_category.edit"); err != nil {
		return nil, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "分类名称不能为空", 400)
		}
		if len([]rune(v)) > 100 {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "分类名称过长", 400)
		}
		in.Name = &v
	}
	if in.Status != nil && !domaincat.IsValidStatus(*in.Status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的分类状态", 400)
	}
	return s.repo.Update(ctx, brandID, actorID, id, domaincat.UpdateInput{
		Name:              in.Name,
		Color:             in.Color,
		Icon:              in.Icon,
		SortOrder:         in.SortOrder,
		ShowInMiniProgram: in.ShowInMiniProgram,
		Status:            in.Status,
	})
}
