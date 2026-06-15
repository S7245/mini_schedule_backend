// Package coursetemplate 课程模板应用服务（Batch 11）。
package coursetemplate

import (
	"context"
	"strings"

	domaintpl "github.com/zkw/mini-schedule/backend/internal/domain/coursetemplate"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker 是 service 需要的最小 Checker 面。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
}

// Service 课程模板应用服务。
type Service struct {
	repo    domaintpl.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限校验（兼容 bootstrap）。
func NewService(repo domaintpl.Repository, checker PermissionChecker) *Service {
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
	Title             string
	Description       string
	CoverURL          string
	LevelLabel        string
	DurationMin       int
	DefaultCapacity   int
	ShowInMiniProgram bool
	CategoryIDs       []int64
	LocationIDs       []int64
}

// UpdateInput 更新入参（白名单）。
type UpdateInput struct {
	Title             *string
	Description       *string
	CoverURL          *string
	LevelLabel        *string
	DurationMin       *int
	DefaultCapacity   *int
	ShowInMiniProgram *bool
	CategoryIDs       *[]int64
	LocationIDs       *[]int64
}

// ListInput 列表查询。
type ListInput struct {
	BrandID    int64
	ActorID    int64
	Status     string
	Q          string
	CategoryID int64
	Page       int
	PageSize   int
}

func validateTitleDuration(title string, durationMin, defaultCapacity int) error {
	t := strings.TrimSpace(title)
	if t == "" {
		return apperr.NewAppError(apperr.ErrInvalidParam, "课程名称不能为空", 400)
	}
	if len([]rune(t)) > 200 {
		return apperr.NewAppError(apperr.ErrInvalidParam, "课程名称过长", 400)
	}
	if durationMin <= 0 {
		return apperr.NewAppError(apperr.ErrInvalidParam, "默认时长必须大于 0", 400)
	}
	if defaultCapacity <= 0 {
		return apperr.NewAppError(apperr.ErrInvalidParam, "默认容量必须大于 0", 400)
	}
	return nil
}

// List 列表（分页 + 状态/名称/分类筛选）。课程模板是品牌级，不做 data_scope 收紧。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domaintpl.Template, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "course.view"); err != nil {
		return nil, 0, err
	}
	page := in.Page
	if page < 1 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	status := in.Status
	if status == "all" {
		status = ""
	}
	return s.repo.List(ctx, domaintpl.ListFilter{
		BrandID:    in.BrandID,
		Status:     status,
		Q:          strings.TrimSpace(in.Q),
		CategoryID: in.CategoryID,
	}, (page-1)*pageSize, pageSize)
}

// Get 详情。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domaintpl.Template, error) {
	if err := s.require(ctx, brandID, actorID, "course.view"); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, brandID, id)
}

// Create 创建课程模板（status=draft）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domaintpl.Template, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "course.create"); err != nil {
		return nil, err
	}
	if err := validateTitleDuration(in.Title, in.DurationMin, in.DefaultCapacity); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, domaintpl.CreateInput{
		BrandID:           in.BrandID,
		ActorID:           in.ActorID,
		Title:             in.Title,
		Description:       in.Description,
		CoverURL:          in.CoverURL,
		LevelLabel:        in.LevelLabel,
		DurationMin:       in.DurationMin,
		DefaultCapacity:   in.DefaultCapacity,
		ShowInMiniProgram: in.ShowInMiniProgram,
		CategoryIDs:       in.CategoryIDs,
		LocationIDs:       in.LocationIDs,
	})
}

// Update 编辑课程模板。
func (s *Service) Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*domaintpl.Template, error) {
	if err := s.require(ctx, brandID, actorID, "course.edit"); err != nil {
		return nil, err
	}
	if in.Title != nil {
		v := strings.TrimSpace(*in.Title)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "课程名称不能为空", 400)
		}
		if len([]rune(v)) > 200 {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "课程名称过长", 400)
		}
		in.Title = &v
	}
	if in.DurationMin != nil && *in.DurationMin <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "默认时长必须大于 0", 400)
	}
	if in.DefaultCapacity != nil && *in.DefaultCapacity <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "默认容量必须大于 0", 400)
	}
	return s.repo.Update(ctx, brandID, actorID, id, domaintpl.UpdateInput{
		Title:             in.Title,
		Description:       in.Description,
		CoverURL:          in.CoverURL,
		LevelLabel:        in.LevelLabel,
		DurationMin:       in.DurationMin,
		DefaultCapacity:   in.DefaultCapacity,
		ShowInMiniProgram: in.ShowInMiniProgram,
		CategoryIDs:       in.CategoryIDs,
		LocationIDs:       in.LocationIDs,
	})
}

// UpdateStatus 状态切换（draft/published/archived）。
func (s *Service) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status string) (*domaintpl.Template, error) {
	if err := s.require(ctx, brandID, actorID, "course.edit"); err != nil {
		return nil, err
	}
	if !domaintpl.IsValidStatus(status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的课程状态", 400)
	}
	return s.repo.UpdateStatus(ctx, brandID, actorID, id, domaintpl.Status(status))
}

// Delete 软删；有 scheduled/in_progress 场次引用时拒删（COURSE_IN_USE）。
func (s *Service) Delete(ctx context.Context, brandID, actorID, id int64) error {
	if err := s.require(ctx, brandID, actorID, "course.delete"); err != nil {
		return err
	}
	// 先确认存在（404 优先于 IN_USE）。
	if _, err := s.repo.GetByID(ctx, brandID, id); err != nil {
		return err
	}
	n, err := s.repo.CountScheduledSessions(ctx, brandID, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return apperr.NewAppError(apperr.ErrCourseInUse, "该课程仍有已排场次，请先取消后再删除", 409)
	}
	return s.repo.SoftDelete(ctx, brandID, actorID, id)
}
