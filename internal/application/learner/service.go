// Package learner 学员档案应用服务（Batch 13a）。
//
// 编排：require(code) + data_scope（学员按 primary_location_id ∈ assigned_locations）+
// 参数校验，落库委托给 repo（quota / find-or-create identity / 唯一冲突分流 / 引用保护在 repo）。
package learner

import (
	"context"
	"strings"

	domainlearner "github.com/zkw/mini-schedule/backend/internal/domain/learner"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker 是 service 需要的最小 Checker 面（Require + Resolve）。
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
}

// Service 学员档案应用服务。
type Service struct {
	repo    domainlearner.Repository
	checker PermissionChecker
}

// NewService 创建 Service。checker == nil 时跳过权限 + data_scope（兼容 bootstrap）。
func NewService(repo domainlearner.Repository, checker PermissionChecker) *Service {
	return &Service{repo: repo, checker: checker}
}

func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// scopeFilterIDs 把 actor 的 data_scope 转为 location id 过滤集（镜像 locationresource）。
// nil = all_brand 不限制；空切片 = 拒绝所有。
func (s *Service) scopeFilterIDs(ctx context.Context, brandID, actorID int64) ([]int64, error) {
	if s.checker == nil {
		return nil, nil
	}
	_, scope, err := s.checker.Resolve(ctx, brandID, actorID)
	if err != nil {
		return nil, err
	}
	switch scope.Kind {
	case domainrbac.DataScopeAllBrand:
		return nil, nil
	case domainrbac.DataScopeAssignedLocations:
		if len(scope.LocationIDs) == 0 {
			return []int64{}, nil
		}
		return scope.LocationIDs, nil
	default:
		return []int64{}, nil
	}
}

func (s *Service) locationInScope(ids []int64, locationID int64) bool {
	for _, lid := range ids {
		if lid == locationID {
			return true
		}
	}
	return false
}

// guardLocationInScope 写/详情路径守卫：assigned_locations 时目标 location 必须在 scope 内，否则 404。
func (s *Service) guardLocationInScope(ctx context.Context, brandID, actorID, locationID int64) error {
	ids, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	if ids == nil {
		return nil // all_brand
	}
	if s.locationInScope(ids, locationID) {
		return nil
	}
	return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
}

// guardProfileScope 守卫已取出的学员：all_brand 放行；assigned 时主门店必须在 scope（无主门店→404）。
func (s *Service) guardProfileScope(ctx context.Context, brandID, actorID int64, primaryLocationID *int64) error {
	ids, err := s.scopeFilterIDs(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	if ids == nil {
		return nil // all_brand
	}
	if primaryLocationID == nil || !s.locationInScope(ids, *primaryLocationID) {
		return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
	}
	return nil
}

func validatePhone(phone string) (string, error) {
	p := strings.TrimSpace(phone)
	if p == "" {
		return "", apperr.NewAppError(apperr.ErrInvalidParam, "手机号不能为空", 400)
	}
	if n := len([]rune(p)); n < 5 || n > 20 {
		return "", apperr.NewAppError(apperr.ErrInvalidParam, "手机号格式不正确", 400)
	}
	return p, nil
}

func validateMaxLen(field, v string, max int) error {
	if len([]rune(strings.TrimSpace(v))) > max {
		return apperr.NewAppError(apperr.ErrInvalidParam, field+"过长", 400)
	}
	return nil
}

// CreateInput 创建入参。
type CreateInput struct {
	BrandID           int64
	ActorID           int64
	Phone             string
	Nickname          string
	PrimaryLocationID *int64
	LearnerNo         string
	Remark            string
	TagIDs            []int64
}

// UpdateInput 编辑入参（白名单）。
type UpdateInput struct {
	Nickname          *string
	PrimaryLocationID *int64
	LearnerNo         *string
	Remark            *string
	TagIDs            *[]int64
}

// ListInput 列表查询。
type ListInput struct {
	BrandID           int64
	ActorID           int64
	Status            string
	PrimaryLocationID int64
	Query             string
	Page              int
	PageSize          int
}

// List 列表（分页 + 过滤 + data_scope）。
func (s *Service) List(ctx context.Context, in ListInput) ([]*domainlearner.Profile, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "learner.view"); err != nil {
		return nil, 0, err
	}
	if in.PrimaryLocationID > 0 {
		if err := s.guardLocationInScope(ctx, in.BrandID, in.ActorID, in.PrimaryLocationID); err != nil {
			return nil, 0, err
		}
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
	scopeIDs, err := s.scopeFilterIDs(ctx, in.BrandID, in.ActorID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, domainlearner.ListFilter{
		BrandID:           in.BrandID,
		Status:            in.Status,
		PrimaryLocationID: in.PrimaryLocationID,
		Query:             in.Query,
		ScopeLocationIDs:  scopeIDs,
	}, (page-1)*pageSize, pageSize)
}

// Get 详情（data_scope 守卫）。
func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*domainlearner.Profile, error) {
	if err := s.require(ctx, brandID, actorID, "learner.view"); err != nil {
		return nil, err
	}
	p, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardProfileScope(ctx, brandID, actorID, p.PrimaryLocationID); err != nil {
		return nil, err
	}
	return p, nil
}

// Create 创建学员。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domainlearner.Profile, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "learner.create"); err != nil {
		return nil, err
	}
	phone, err := validatePhone(in.Phone)
	if err != nil {
		return nil, err
	}
	if err := validateMaxLen("昵称", in.Nickname, 100); err != nil {
		return nil, err
	}
	if err := validateMaxLen("学号", in.LearnerNo, 50); err != nil {
		return nil, err
	}
	if err := validateMaxLen("备注", in.Remark, 1000); err != nil {
		return nil, err
	}
	// data_scope：指定主门店须在 scope 内；assigned_locations 员工必须指定主门店（否则学员对其不可见）。
	if in.PrimaryLocationID != nil && *in.PrimaryLocationID > 0 {
		if err := s.guardLocationInScope(ctx, in.BrandID, in.ActorID, *in.PrimaryLocationID); err != nil {
			return nil, err
		}
	} else {
		ids, err := s.scopeFilterIDs(ctx, in.BrandID, in.ActorID)
		if err != nil {
			return nil, err
		}
		if ids != nil {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "请为学员指定主门店", 400)
		}
	}
	return s.repo.Create(ctx, domainlearner.CreateInput{
		BrandID:           in.BrandID,
		ActorID:           in.ActorID,
		Phone:             phone,
		Nickname:          strings.TrimSpace(in.Nickname),
		PrimaryLocationID: in.PrimaryLocationID,
		LearnerNo:         in.LearnerNo,
		Remark:            in.Remark,
		TagIDs:            in.TagIDs,
	})
}

// Update 编辑学员。
func (s *Service) Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*domainlearner.Profile, error) {
	if err := s.require(ctx, brandID, actorID, "learner.edit"); err != nil {
		return nil, err
	}
	if in.Nickname != nil {
		if err := validateMaxLen("昵称", *in.Nickname, 100); err != nil {
			return nil, err
		}
	}
	if in.LearnerNo != nil {
		if err := validateMaxLen("学号", *in.LearnerNo, 50); err != nil {
			return nil, err
		}
	}
	if in.Remark != nil {
		if err := validateMaxLen("备注", *in.Remark, 1000); err != nil {
			return nil, err
		}
	}
	// 既有学员须在 scope 内才可编辑。
	existing, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardProfileScope(ctx, brandID, actorID, existing.PrimaryLocationID); err != nil {
		return nil, err
	}
	// 改主门店：新门店也须在 scope 内。
	if in.PrimaryLocationID != nil && *in.PrimaryLocationID > 0 {
		if err := s.guardLocationInScope(ctx, brandID, actorID, *in.PrimaryLocationID); err != nil {
			return nil, err
		}
	}
	return s.repo.Update(ctx, brandID, actorID, id, domainlearner.UpdateInput{
		Nickname:          in.Nickname,
		PrimaryLocationID: in.PrimaryLocationID,
		LearnerNo:         in.LearnerNo,
		Remark:            in.Remark,
		TagIDs:            in.TagIDs,
	})
}

// UpdateStatus 冻结 / 解冻（仅 active ↔ frozen）。
func (s *Service) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status string) (*domainlearner.Profile, error) {
	if err := s.require(ctx, brandID, actorID, "learner.freeze"); err != nil {
		return nil, err
	}
	if status != string(domainlearner.StatusActive) && status != string(domainlearner.StatusFrozen) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的学员状态", 400)
	}
	existing, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}
	if err := s.guardProfileScope(ctx, brandID, actorID, existing.PrimaryLocationID); err != nil {
		return nil, err
	}
	return s.repo.UpdateStatus(ctx, brandID, actorID, id, status)
}

// Delete 软删学员（引用保护在 repo）。
func (s *Service) Delete(ctx context.Context, brandID, actorID, id int64) error {
	if err := s.require(ctx, brandID, actorID, "learner.delete"); err != nil {
		return err
	}
	existing, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return err
	}
	if err := s.guardProfileScope(ctx, brandID, actorID, existing.PrimaryLocationID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, brandID, actorID, id)
}

// ---- 标签（品牌级，无 location scope）----

// CreateTagInput 创建标签入参。
type CreateTagInput struct {
	BrandID int64
	ActorID int64
	Name    string
	Color   string
}

// UpdateTagInput 编辑标签入参。
type UpdateTagInput struct {
	Name   *string
	Color  *string
	Status *string
}

// ListTags 标签列表。
func (s *Service) ListTags(ctx context.Context, brandID, actorID int64, status string) ([]*domainlearner.Tag, error) {
	if err := s.require(ctx, brandID, actorID, "learner.view"); err != nil {
		return nil, err
	}
	return s.repo.ListTags(ctx, domainlearner.TagListFilter{BrandID: brandID, Status: status})
}

// CreateTag 创建标签。
func (s *Service) CreateTag(ctx context.Context, in CreateTagInput) (*domainlearner.Tag, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "learner.edit"); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "标签名不能为空", 400)
	}
	if err := validateMaxLen("标签名", name, 50); err != nil {
		return nil, err
	}
	return s.repo.CreateTag(ctx, domainlearner.CreateTagInput{BrandID: in.BrandID, ActorID: in.ActorID, Name: name, Color: in.Color})
}

// UpdateTag 编辑标签（含状态切换）。
func (s *Service) UpdateTag(ctx context.Context, brandID, actorID, id int64, in UpdateTagInput) (*domainlearner.Tag, error) {
	if err := s.require(ctx, brandID, actorID, "learner.edit"); err != nil {
		return nil, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "标签名不能为空", 400)
		}
		if err := validateMaxLen("标签名", v, 50); err != nil {
			return nil, err
		}
		in.Name = &v
	}
	if in.Status != nil && !domainlearner.IsValidTagStatus(*in.Status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "无效的标签状态", 400)
	}
	return s.repo.UpdateTag(ctx, brandID, actorID, id, domainlearner.UpdateTagInput{Name: in.Name, Color: in.Color, Status: in.Status})
}
