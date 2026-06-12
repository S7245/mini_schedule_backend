// Package staff 应用层。
//
// 编排：参数校验 + Subscription quota（通过 SubscriptionGuard）+
// Owner 保护 + 角色 / Location 任职管理 + 教练档案 1:1 upsert。
package staff

import (
	"context"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/zkw/mini-schedule/backend/internal/domain/instructor"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	"github.com/zkw/mini-schedule/backend/internal/domain/staff"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// PermissionChecker is the minimal Checker surface staff.Service needs. Defined
// here (rather than imported from application/rbac) to keep this package free of
// the rbac package's transitive Redis dependency in tests.
type PermissionChecker interface {
	Require(ctx context.Context, brandID, brandUserID int64, code string) error
	Resolve(ctx context.Context, brandID, brandUserID int64) (domainrbac.PermissionSet, domainrbac.DataScope, error)
	// Invalidate evicts the cached permission set for a brand_user (Batch 7 C1).
	Invalidate(ctx context.Context, brandUserID int64) error
}

// Service 编排 staff CRUD + 角色 / 任职 / 教练。
//
// 注：subscription quota 校验下沉到 staff_repository.Create 的事务内，
// 复用 location_repository 同款 SubscriptionGuard 模板。
//
// Batch 6: checker 注入用于每个 method 头部 RequirePermission 校验。
// nil checker（仅注册流程 / system-internal 路径）会跳过校验。
type Service struct {
	repo           staff.Repository
	roleRepo       role.Repository
	instructorRepo instructor.Repository
	checker        PermissionChecker
}

// NewService 构造函数（Wire 会注入）。
func NewService(
	repo staff.Repository,
	roleRepo role.Repository,
	instructorRepo instructor.Repository,
	checker PermissionChecker,
) *Service {
	return &Service{
		repo:           repo,
		roleRepo:       roleRepo,
		instructorRepo: instructorRepo,
		checker:        checker,
	}
}

// require 包装 checker.Require，checker == nil 时跳过（兼容旧测试 / 引导调用）。
func (s *Service) require(ctx context.Context, brandID, actorID int64, code string) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Require(ctx, brandID, actorID, code)
}

// resolveScope 拿当前 actor 的 DataScope（owner / all_brand → nil，其余转换为
// location_ids 集合给 repo 过滤）。
func (s *Service) resolveScope(ctx context.Context, brandID, actorID int64) (*domainrbac.DataScope, error) {
	if s.checker == nil {
		return nil, nil
	}
	_, scope, err := s.checker.Resolve(ctx, brandID, actorID)
	if err != nil {
		return nil, err
	}
	return &scope, nil
}

// scopeFilterIDs 把 DataScope 转换为 ListFilter.ScopeLocationIDs 语义：
// nil（checker 缺省 / all_brand）→ nil 不限制；assigned_locations → ids；none → 空切片拒绝所有。
func scopeFilterIDs(scope *domainrbac.DataScope) []int64 {
	if scope == nil || scope.Kind == domainrbac.DataScopeAllBrand {
		return nil
	}
	if scope.Kind == domainrbac.DataScopeAssignedLocations {
		if len(scope.LocationIDs) == 0 {
			return []int64{}
		}
		return scope.LocationIDs
	}
	// DataScopeNone 或未知 → 拒绝所有
	return []int64{}
}

// guardTargetInScope 详情/写路径守卫：assigned_locations 时目标 staff 必须任职在 scope 内，
// 否则按"不可见"返 404（per 契约决定 4：跨 scope 用 404 隐藏存在性）。
func (s *Service) guardTargetInScope(ctx context.Context, brandID, actorID, targetID int64) error {
	scope, err := s.resolveScope(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	ids := scopeFilterIDs(scope)
	if ids == nil {
		return nil // all_brand
	}
	ok, err := s.repo.InScopeLocations(ctx, brandID, targetID, ids)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
	}
	return nil
}

// Public input types（handler 直接构造，不经 domain）。

type CreateInput struct {
	BrandID             int64
	ActorID             int64
	Phone               string
	Name                string
	InitialPassword     string
	RoleCodes           []string
	LocationAssignments []staff.LocationAssignmentInput
}

type UpdateInput = staff.UpdateInput

type ListInput struct {
	BrandID       int64
	ActorID       int64
	Status        string
	HasInstructor *bool
	Search        string
	Page          int
	PageSize      int
}

var (
	phoneRegex    = regexp.MustCompile(`^\+?\d{6,20}$`)
	passwordLen   = 8
	passwordRules = regexp.MustCompile(`^[A-Za-z\d!@#$%^&*()_+\-=\[\]{};':"\\|,.<>/?~]{8,64}$`)
)

func (s *Service) Create(ctx context.Context, in CreateInput) (*staff.Staff, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "staff.create"); err != nil {
		return nil, err
	}
	if in.BrandID <= 0 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "品牌 ID 无效", 400)
	}
	if !phoneRegex.MatchString(strings.TrimSpace(in.Phone)) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "手机号格式不合法", 400)
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "姓名不能为空", 400)
	}
	if len([]rune(in.Name)) > 50 {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "姓名过长", 400)
	}
	if err := validatePassword(in.InitialPassword); err != nil {
		return nil, err
	}

	// 1) 校验 location_assignments 不重复 + 最多一个 primary
	if err := validateLocationAssignments(in.LocationAssignments); err != nil {
		return nil, err
	}

	// 2) 解析 role_codes → role_id（提前校验，避免 quota 占了再回滚）
	roleResolved, err := s.resolveRoleAssignments(ctx, in.BrandID, in.RoleCodes, in.LocationAssignments)
	if err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.InitialPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.ErrInternalF("加密密码失败", err)
	}

	// 3) 建 staff（仓储内事务：guard + INSERT brand_user + audit.staff_created）。
	created, err := s.repo.Create(ctx, staff.CreateInput{
		BrandID:         in.BrandID,
		ActorID:         in.ActorID,
		Phone:           in.Phone,
		Name:            in.Name,
		InitialPassword: string(hash),
	})
	if err != nil {
		return nil, err
	}
	staffID := created.ID

	// 5) 角色任职
	if len(roleResolved) > 0 {
		if _, err := s.repo.ReplaceRoleAssignments(ctx, in.BrandID, in.ActorID, staffID, roleResolved); err != nil {
			return nil, err
		}
	}
	// 6) Location 任职
	if len(in.LocationAssignments) > 0 {
		if _, err := s.repo.ReplaceLocationAssignments(ctx, in.BrandID, in.ActorID, staffID, in.LocationAssignments); err != nil {
			return nil, err
		}
	}

	return s.repo.GetWithAssignments(ctx, in.BrandID, staffID)
}

func validatePassword(p string) error {
	if len(p) < passwordLen || len(p) > 64 {
		return apperr.NewAppError(apperr.ErrInvalidParam, "密码长度需 8-64 位", 400)
	}
	if !passwordRules.MatchString(p) {
		return apperr.NewAppError(apperr.ErrInvalidParam, "密码格式不合法", 400)
	}
	hasLetter, hasDigit := false, false
	for _, c := range p {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			hasLetter = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return apperr.NewAppError(apperr.ErrInvalidParam, "密码需同时包含字母与数字", 400)
	}
	return nil
}

func validateLocationAssignments(items []staff.LocationAssignmentInput) error {
	primary := 0
	seen := map[int64]bool{}
	for _, it := range items {
		if seen[it.LocationID] {
			return apperr.NewAppError(apperr.ErrLocationAssignmentInvalid, "门店任职重复", 400)
		}
		seen[it.LocationID] = true

		switch it.AssignmentType {
		case "member", "manager", "instructor", "assistant":
		default:
			return apperr.NewAppError(apperr.ErrInvalidParam, "门店任职类型不合法", 400)
		}
		if it.IsPrimary {
			primary++
		}
	}
	if primary > 1 {
		return apperr.NewAppError(apperr.ErrInvalidParam, "最多 1 个门店任职可设为主要", 400)
	}
	return nil
}

// resolveRoleAssignments 把 role_codes / location_assignments 翻译成 role_resolved 列表。
// 校验：role_code 存在、scope_type 与 location_id 匹配、禁止手动分配 brand_owner。
func (s *Service) resolveRoleAssignments(
	ctx context.Context,
	brandID int64,
	roleCodes []string,
	locAssigns []staff.LocationAssignmentInput,
) ([]staff.RoleAssignmentResolved, error) {
	if len(roleCodes) == 0 {
		return nil, nil
	}
	// 主 location_id 用于 location-scope 角色
	var primaryLocID *int64
	for i, la := range locAssigns {
		if la.IsPrimary {
			id := locAssigns[i].LocationID
			primaryLocID = &id
			break
		}
	}

	out := make([]staff.RoleAssignmentResolved, 0, len(roleCodes))
	for _, code := range roleCodes {
		if code == "brand_owner" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "品牌负责人角色不可手动分配", 400)
		}
		br, err := s.roleRepo.GetBrandRoleByCode(ctx, brandID, code)
		if err != nil {
			return nil, err
		}
		if br.Status != "active" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "角色已停用："+code, 400)
		}
		resolved := staff.RoleAssignmentResolved{
			RoleID:    br.ID,
			ScopeType: br.ScopeType,
			DataScope: role.DataScopeRoleDefault,
		}
		if br.ScopeType == role.ScopeLocation {
			if primaryLocID == nil {
				return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店级角色需要至少 1 个 location 任职并指定 primary", 400)
			}
			resolved.LocationID = primaryLocID
		}
		out = append(out, resolved)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, brandID, actorID, id int64) (*staff.Staff, error) {
	if err := s.require(ctx, brandID, actorID, "staff.view"); err != nil {
		return nil, err
	}
	// Batch 6 T07：assigned_locations 时目标不在 scope → 404
	if err := s.guardTargetInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	return s.repo.GetWithAssignments(ctx, brandID, id)
}

func (s *Service) List(ctx context.Context, in ListInput) ([]*staff.Staff, int64, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "staff.view"); err != nil {
		return nil, 0, err
	}
	// Batch 6 T07：拿 actor 的 data_scope 收紧列表
	scope, err := s.resolveScope(ctx, in.BrandID, in.ActorID)
	if err != nil {
		return nil, 0, err
	}
	page := in.Page
	if page < 1 {
		page = 1
	}
	size := in.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return s.repo.List(ctx, staff.ListFilter{
		BrandID:          in.BrandID,
		Status:           in.Status,
		HasInstructor:    in.HasInstructor,
		Search:           in.Search,
		ScopeLocationIDs: scopeFilterIDs(scope),
	}, (page-1)*size, size)
}

func (s *Service) Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*staff.Staff, error) {
	if err := s.require(ctx, brandID, actorID, "staff.edit"); err != nil {
		return nil, err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "姓名不能为空", 400)
		}
		if len([]rune(v)) > 50 {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "姓名过长", 400)
		}
		in.Name = &v
	}
	return s.repo.Update(ctx, brandID, actorID, id, in)
}

// UpdateStatus 切换 active / inactive。Owner 不可置 inactive。
func (s *Service) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status string) (*staff.Staff, error) {
	if err := s.require(ctx, brandID, actorID, "staff.edit"); err != nil {
		return nil, err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	if !staff.IsValidStatus(status) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "员工状态无效", 400)
	}
	if status == string(staff.StatusInactive) {
		if err := s.ensureNotLastActiveOwner(ctx, brandID, id); err != nil {
			return nil, err
		}
	}
	return s.repo.UpdateStatus(ctx, brandID, actorID, id, staff.Status(status))
}

func (s *Service) Delete(ctx context.Context, brandID, actorID, id int64) error {
	if err := s.require(ctx, brandID, actorID, "staff.delete"); err != nil {
		return err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, id); err != nil {
		return err
	}
	if err := s.ensureNotLastActiveOwner(ctx, brandID, id); err != nil {
		return err
	}
	return s.repo.SoftDelete(ctx, brandID, actorID, id)
}

// ensureNotLastActiveOwner 查目标是否 owner；如果是 owner 且 active_owner_count(brand)==1 → 拒。
func (s *Service) ensureNotLastActiveOwner(ctx context.Context, brandID, id int64) error {
	target, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return err
	}
	if !target.IsOwner {
		return nil
	}
	count, err := s.repo.CountActiveOwners(ctx, brandID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return apperr.NewAppError(apperr.ErrOwnerProtected, "不能删除/停用唯一的品牌负责人", 409)
	}
	return nil
}

// ReplaceRoleAssignments PUT /staff/:id/role-assignments
func (s *Service) ReplaceRoleAssignments(
	ctx context.Context, brandID, actorID, id int64, items []staff.RoleAssignmentInput,
) (*staff.Staff, error) {
	if err := s.require(ctx, brandID, actorID, "staff.assign_role"); err != nil {
		return nil, err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	// 校验目标 staff 存在
	target, err := s.repo.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, err
	}

	// review B4：owner 的角色集合不可被 PUT 修改。owner 的 brand_owner 关联只能由 backfill /
	// 注册流程维护；如果允许此处替换，brand_admin 一调用就能静默清掉 owner 的全部权限
	// （包括 brand_owner）— 是事实上的权限提升 + 否认服务漏洞。
	if target.IsOwner {
		return nil, apperr.NewAppError(apperr.ErrOwnerProtected, "品牌负责人的角色不可手动修改", 409)
	}

	// 校验 + 解析每行
	resolved := make([]staff.RoleAssignmentResolved, 0, len(items))
	for _, it := range items {
		if it.RoleCode == "brand_owner" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "品牌负责人角色不可手动分配", 400)
		}
		br, err := s.roleRepo.GetBrandRoleByCode(ctx, brandID, it.RoleCode)
		if err != nil {
			return nil, err
		}
		if br.Status != "active" {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "角色已停用："+it.RoleCode, 400)
		}
		if br.ScopeType == role.ScopeLocation && it.LocationID == nil {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "门店级角色需 location_id", 400)
		}
		if br.ScopeType == role.ScopeBrand && it.LocationID != nil {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "品牌级角色不可绑定 location", 400)
		}
		ds := it.DataScope
		if ds == "" {
			ds = role.DataScopeRoleDefault
		}
		if !role.IsValidDataScope(ds) {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "data_scope 不合法", 400)
		}
		resolved = append(resolved, staff.RoleAssignmentResolved{
			RoleID:     br.ID,
			ScopeType:  br.ScopeType,
			LocationID: it.LocationID,
			DataScope:  ds,
		})
	}
	if _, err := s.repo.ReplaceRoleAssignments(ctx, brandID, actorID, id, resolved); err != nil {
		return nil, err
	}
	return s.repo.GetWithAssignments(ctx, brandID, id)
}

// ReplaceLocationAssignments PUT /staff/:id/location-assignments
func (s *Service) ReplaceLocationAssignments(
	ctx context.Context, brandID, actorID, id int64, items []staff.LocationAssignmentInput,
) (*staff.Staff, error) {
	if err := s.require(ctx, brandID, actorID, "staff.assign_location"); err != nil {
		return nil, err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, id); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, brandID, id); err != nil {
		return nil, err
	}
	if err := validateLocationAssignments(items); err != nil {
		return nil, err
	}
	if _, err := s.repo.ReplaceLocationAssignments(ctx, brandID, actorID, id, items); err != nil {
		return nil, err
	}
	return s.repo.GetWithAssignments(ctx, brandID, id)
}

// GetInstructor 获取教练档案；如果 staff 跨 brand 则 404 STAFF_NOT_FOUND。
func (s *Service) GetInstructor(ctx context.Context, brandID, actorID, staffID int64) (*instructor.Profile, error) {
	if err := s.require(ctx, brandID, actorID, "instructor.view"); err != nil {
		return nil, err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, staffID); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, brandID, staffID); err != nil {
		return nil, err
	}
	return s.instructorRepo.GetByBrandUserID(ctx, brandID, staffID)
}

// UpsertInstructor 编辑教练档案。
func (s *Service) UpsertInstructor(ctx context.Context, brandID, actorID, staffID int64, in instructor.UpsertInput) (*instructor.Profile, error) {
	if err := s.require(ctx, brandID, actorID, "instructor.edit"); err != nil {
		return nil, err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, staffID); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, brandID, staffID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "教练姓名不能为空", 400)
	}
	if in.Status == "" {
		in.Status = instructor.StatusActive
	}
	if !instructor.IsValidStatus(string(in.Status)) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "教练状态无效", 400)
	}
	in.BrandID = brandID
	in.BrandUserID = staffID
	return s.instructorRepo.Upsert(ctx, actorID, in)
}

// DeleteInstructor 注销教练档案。
func (s *Service) DeleteInstructor(ctx context.Context, brandID, actorID, staffID int64) error {
	if err := s.require(ctx, brandID, actorID, "instructor.edit"); err != nil {
		return err
	}
	if err := s.guardTargetInScope(ctx, brandID, actorID, staffID); err != nil {
		return err
	}
	if _, err := s.repo.GetByID(ctx, brandID, staffID); err != nil {
		return err
	}
	return s.instructorRepo.Delete(ctx, brandID, actorID, staffID)
}

// ListRoles 暴露给 GET /brand/roles。
func (s *Service) ListRoles(ctx context.Context, brandID, actorID int64) ([]*role.BrandRole, error) {
	if err := s.require(ctx, brandID, actorID, "staff.view"); err != nil {
		return nil, err
	}
	return s.roleRepo.ListBrandRoles(ctx, brandID)
}

// GetRole 单角色详情，含 permissions。
func (s *Service) GetRole(ctx context.Context, brandID, actorID int64, code string) (*role.BrandRole, error) {
	if err := s.require(ctx, brandID, actorID, "staff.view"); err != nil {
		return nil, err
	}
	br, err := s.roleRepo.GetBrandRoleByCode(ctx, brandID, code)
	if err != nil {
		return nil, err
	}
	// Fetch permissions for this role via the list path; cheap because we know
	// the single role's permission set is loaded eagerly by ListBrandRoles, but
	// we don't have a single-role-with-perms accessor. Refetch via list filter.
	roles, err := s.roleRepo.ListBrandRoles(ctx, brandID)
	if err != nil {
		return br, nil
	}
	for _, r := range roles {
		if r.Code == code {
			return r, nil
		}
	}
	return br, nil
}

// roleNameMax 角色名长度上限（契约：1–40 字符）。
const roleNameMax = 40

// CreateRoleInput POST /brand/roles 入参（handler 构造）。
type CreateRoleInput struct {
	BrandID         int64
	ActorID         int64
	Name            string
	ScopeType       string
	Description     string
	PermissionCodes []string
}

// UpdateRoleInput PUT /brand/roles/:code 入参（scope_type 不接受，A3）。
type UpdateRoleInput struct {
	BrandID         int64
	ActorID         int64
	Code            string
	Name            string
	Description     string
	PermissionCodes []string
}

// ListPermissions GET /brand/permissions —— 全量细粒度权限（gate role.manage）。
func (s *Service) ListPermissions(ctx context.Context, brandID, actorID int64) ([]role.Permission, error) {
	if err := s.require(ctx, brandID, actorID, "role.manage"); err != nil {
		return nil, err
	}
	return s.roleRepo.ListPermissions(ctx)
}

// CreateRole 新建自定义角色（gate role.manage）。
// B1：非 owner 时 permission_codes 必须 ⊆ actor 有效权限集，否则 ROLE_PERMISSION_EXCEEDS_ACTOR。
func (s *Service) CreateRole(ctx context.Context, in CreateRoleInput) (*role.BrandRole, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "role.manage"); err != nil {
		return nil, err
	}
	if err := validateRoleName(in.Name); err != nil {
		return nil, err
	}
	if in.ScopeType != role.ScopeBrand && in.ScopeType != role.ScopeLocation {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "scope_type 不合法", 400)
	}
	if err := s.guardPermissionSubset(ctx, in.BrandID, in.ActorID, in.PermissionCodes); err != nil {
		return nil, err
	}
	created, err := s.roleRepo.CreateBrandRole(ctx, role.CreateBrandRoleInput{
		BrandID:         in.BrandID,
		ActorID:         in.ActorID,
		Name:            strings.TrimSpace(in.Name),
		ScopeType:       in.ScopeType,
		Description:     strings.TrimSpace(in.Description),
		PermissionCodes: in.PermissionCodes,
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateRole 编辑自定义角色（gate role.manage）。
// 拦截 is_system / brand_owner；B1 提权校验；scope_type 不可改（A3，入参里没有 scope_type）。
// C1：成功后 post-commit 失效持有该角色的全部 brand_user 缓存。
func (s *Service) UpdateRole(ctx context.Context, in UpdateRoleInput) (*role.BrandRole, error) {
	if err := s.require(ctx, in.BrandID, in.ActorID, "role.manage"); err != nil {
		return nil, err
	}
	if err := validateRoleName(in.Name); err != nil {
		return nil, err
	}
	target, err := s.requireMutableCustomRole(ctx, in.BrandID, in.Code)
	if err != nil {
		return nil, err
	}
	// B1（增量语义）：只对"新增"的权限做 ⊆ actor 校验；角色原有的权限可保留/移除，
	// 不因 actor 自身缺该权限而被拦——既守住"不能授予自己没有的权限"的提权防线，
	// 又不让受限管理员连改名都被卡死。create 全是新增，故仍走全集校验。
	existing, err := s.roleRepo.ListRolePermissionCodes(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	if err := s.guardPermissionSubset(ctx, in.BrandID, in.ActorID, addedCodes(in.PermissionCodes, existing)); err != nil {
		return nil, err
	}
	updated, err := s.roleRepo.UpdateBrandRole(ctx, role.UpdateBrandRoleInput{
		BrandID:         in.BrandID,
		ActorID:         in.ActorID,
		RoleID:          target.ID,
		Name:            strings.TrimSpace(in.Name),
		Description:     strings.TrimSpace(in.Description),
		PermissionCodes: in.PermissionCodes,
	})
	if err != nil {
		return nil, err
	}
	s.invalidateRoleHolders(ctx, target.ID)
	return updated, nil
}

// PatchRoleStatus PATCH /brand/roles/:code/status（gate role.manage）。
func (s *Service) PatchRoleStatus(ctx context.Context, brandID, actorID int64, code, status string) (*role.BrandRole, error) {
	if err := s.require(ctx, brandID, actorID, "role.manage"); err != nil {
		return nil, err
	}
	if status != "active" && status != "inactive" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "状态不合法", 400)
	}
	target, err := s.requireMutableCustomRole(ctx, brandID, code)
	if err != nil {
		return nil, err
	}
	updated, err := s.roleRepo.UpdateBrandRoleStatus(ctx, brandID, actorID, target.ID, status)
	if err != nil {
		return nil, err
	}
	s.invalidateRoleHolders(ctx, target.ID)
	return updated, nil
}

// DeleteRole DELETE /brand/roles/:code（gate role.manage）。
// A4：仍有 active 任职引用时拒删 → ROLE_IN_USE。
func (s *Service) DeleteRole(ctx context.Context, brandID, actorID int64, code string) error {
	if err := s.require(ctx, brandID, actorID, "role.manage"); err != nil {
		return err
	}
	target, err := s.requireMutableCustomRole(ctx, brandID, code)
	if err != nil {
		return err
	}
	count, err := s.roleRepo.CountAssignmentsByRole(ctx, target.ID)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperr.NewAppError(apperr.ErrRoleInUse, "该角色仍有员工任职，请先移除", 409)
	}
	// 走到这里 count==0：无任职引用 → 无缓存持有人，删后无需失效任何用户缓存。
	return s.roleRepo.DeleteBrandRole(ctx, brandID, actorID, target.ID)
}

// requireMutableCustomRole 取角色并拦截系统角色 / owner 系统角色（A1/D2）。
func (s *Service) requireMutableCustomRole(ctx context.Context, brandID int64, code string) (*role.BrandRole, error) {
	br, err := s.roleRepo.GetBrandRoleByCode(ctx, brandID, code)
	if err != nil {
		return nil, err
	}
	if br.Code == "brand_owner" {
		return nil, apperr.NewAppError(apperr.ErrOwnerProtected, "品牌负责人角色受保护，不可修改或删除", 409)
	}
	if br.IsSystem {
		return nil, apperr.NewAppError(apperr.ErrRoleIsSystem, "系统角色只读，不可修改或删除", 409)
	}
	return br, nil
}

// addedCodes 返回 want 中不在 have 里的 code（即本次新增的权限）。
func addedCodes(want, have []string) []string {
	existing := make(map[string]struct{}, len(have))
	for _, c := range have {
		existing[c] = struct{}{}
	}
	added := make([]string, 0)
	for _, c := range want {
		if _, ok := existing[c]; !ok {
			added = append(added, c)
		}
	}
	return added
}

// guardPermissionSubset B1：非 owner 时勾选权限必须 ⊆ actor 有效权限集。
// owner（is_owner=TRUE）跳过。checker == nil（引导路径）跳过。
func (s *Service) guardPermissionSubset(ctx context.Context, brandID, actorID int64, codes []string) error {
	if s.checker == nil || len(codes) == 0 {
		return nil
	}
	actor, err := s.repo.GetByID(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	if actor.IsOwner {
		return nil
	}
	effective, _, err := s.checker.Resolve(ctx, brandID, actorID)
	if err != nil {
		return err
	}
	for _, c := range codes {
		if !effective.Has(c) {
			return apperr.NewAppError(apperr.ErrRolePermissionExceedsActor, "勾选的权限超出你自身的权限范围", 403).
				WithDetails(map[string]any{"exceeded": c})
		}
	}
	return nil
}

// invalidateRoleHolders C1：反查持有该角色的全部 brand_user 并逐一失效缓存（post-commit）。
func (s *Service) invalidateRoleHolders(ctx context.Context, roleID int64) {
	if s.checker == nil {
		return
	}
	ids, err := s.roleRepo.ListBrandUserIDsByRole(ctx, roleID)
	if err != nil {
		return // 失效失败不阻断主流程；60s TTL 兜底
	}
	for _, uid := range ids {
		_ = s.invalidateUser(ctx, uid)
	}
}

func (s *Service) invalidateUser(ctx context.Context, brandUserID int64) error {
	if s.checker == nil {
		return nil
	}
	return s.checker.Invalidate(ctx, brandUserID)
}

func validateRoleName(name string) error {
	v := strings.TrimSpace(name)
	if v == "" {
		return apperr.NewAppError(apperr.ErrInvalidParam, "角色名不能为空", 400)
	}
	if len([]rune(v)) > roleNameMax {
		return apperr.NewAppError(apperr.ErrInvalidParam, "角色名过长", 400)
	}
	return nil
}
