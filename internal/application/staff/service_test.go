package staff

import (
	"context"
	"testing"

	"github.com/zkw/mini-schedule/backend/internal/domain/instructor"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	"github.com/zkw/mini-schedule/backend/internal/domain/staff"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// ---- fakes ----

type fakeStaffRepo struct {
	createIn       *staff.CreateInput
	created        *staff.Staff
	createErr      error
	getByID        *staff.Staff
	getByIDErr     error
	getAssign      *staff.Staff
	getAssignErr   error
	listItems      []*staff.Staff
	listTotal      int64
	updErr         error
	updStatusErr   error
	delErr         error
	ownerCount     int64
	ownerCountErr  error
	roleReplaced   []staff.RoleAssignmentResolved
	roleReplaceErr error
	locReplaced    []staff.LocationAssignmentInput
	locReplaceErr  error
}

func (f *fakeStaffRepo) Create(_ context.Context, in staff.CreateInput) (*staff.Staff, error) {
	f.createIn = &in
	return f.created, f.createErr
}
func (f *fakeStaffRepo) GetByID(_ context.Context, _, _ int64) (*staff.Staff, error) {
	return f.getByID, f.getByIDErr
}
func (f *fakeStaffRepo) GetWithAssignments(_ context.Context, _, _ int64) (*staff.Staff, error) {
	return f.getAssign, f.getAssignErr
}
func (f *fakeStaffRepo) List(_ context.Context, _ staff.ListFilter, _, _ int) ([]*staff.Staff, int64, error) {
	return f.listItems, f.listTotal, nil
}
func (f *fakeStaffRepo) Update(_ context.Context, _, _, _ int64, _ staff.UpdateInput) (*staff.Staff, error) {
	return f.getAssign, f.updErr
}
func (f *fakeStaffRepo) UpdateStatus(_ context.Context, _, _, _ int64, _ staff.Status) (*staff.Staff, error) {
	return f.getAssign, f.updStatusErr
}
func (f *fakeStaffRepo) SoftDelete(_ context.Context, _, _, _ int64) error {
	return f.delErr
}
func (f *fakeStaffRepo) CountActiveOwners(_ context.Context, _ int64) (int64, error) {
	return f.ownerCount, f.ownerCountErr
}
func (f *fakeStaffRepo) InScopeLocations(_ context.Context, _, _ int64, _ []int64) (bool, error) {
	return true, nil // 单测默认 in-scope；scope 收紧专项测试用独立 fake 覆盖
}
func (f *fakeStaffRepo) ReplaceRoleAssignments(_ context.Context, _, _, _ int64, items []staff.RoleAssignmentResolved) ([]staff.RoleAssignment, error) {
	f.roleReplaced = items
	return nil, f.roleReplaceErr
}
func (f *fakeStaffRepo) ReplaceLocationAssignments(_ context.Context, _, _, _ int64, items []staff.LocationAssignmentInput) ([]staff.LocationAssignment, error) {
	f.locReplaced = items
	return nil, f.locReplaceErr
}

type fakeRoleRepo struct {
	byCode    map[string]*role.BrandRole
	byCodeErr error
	templates []*role.RoleTemplate

	// Batch 7 — custom role CRUD fakes.
	permissions       []role.Permission
	permissionsErr    error
	createIn          *role.CreateBrandRoleInput
	createOut         *role.BrandRole
	createErr         error
	updateIn          *role.UpdateBrandRoleInput
	updateOut         *role.BrandRole
	updateErr         error
	statusOut         *role.BrandRole
	statusErr         error
	statusSeen        string
	deleteErr         error
	deleteSeen        int64
	activeAssignments int64
	activeAssignErr   error
	userIDsByRole     []int64
	userIDsErr        error
	existingPermCodes []string
	existingPermErr   error
}

func (f *fakeRoleRepo) ListPermissions(_ context.Context) ([]role.Permission, error) {
	return f.permissions, f.permissionsErr
}
func (f *fakeRoleRepo) CreateBrandRole(_ context.Context, in role.CreateBrandRoleInput) (*role.BrandRole, error) {
	f.createIn = &in
	return f.createOut, f.createErr
}
func (f *fakeRoleRepo) UpdateBrandRole(_ context.Context, in role.UpdateBrandRoleInput) (*role.BrandRole, error) {
	f.updateIn = &in
	return f.updateOut, f.updateErr
}
func (f *fakeRoleRepo) UpdateBrandRoleStatus(_ context.Context, _, _, _ int64, status string) (*role.BrandRole, error) {
	f.statusSeen = status
	return f.statusOut, f.statusErr
}
func (f *fakeRoleRepo) DeleteBrandRole(_ context.Context, _, _, roleID int64) error {
	f.deleteSeen = roleID
	return f.deleteErr
}
func (f *fakeRoleRepo) CountAssignmentsByRole(_ context.Context, _ int64) (int64, error) {
	return f.activeAssignments, f.activeAssignErr
}
func (f *fakeRoleRepo) ListBrandUserIDsByRole(_ context.Context, _ int64) ([]int64, error) {
	return f.userIDsByRole, f.userIDsErr
}
func (f *fakeRoleRepo) ListRolePermissionCodes(_ context.Context, _ int64) ([]string, error) {
	return f.existingPermCodes, f.existingPermErr
}

func (f *fakeRoleRepo) ListBrandRoles(_ context.Context, _ int64) ([]*role.BrandRole, error) {
	var out []*role.BrandRole
	for _, v := range f.byCode {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeRoleRepo) GetBrandRoleByCode(_ context.Context, _ int64, code string) (*role.BrandRole, error) {
	if f.byCodeErr != nil {
		return nil, f.byCodeErr
	}
	if v, ok := f.byCode[code]; ok {
		return v, nil
	}
	return nil, apperr.NewAppError(apperr.ErrRoleNotFound, "x", 404)
}
func (f *fakeRoleRepo) GetBrandRoleWithPermissions(_ context.Context, _ int64, code string) (*role.BrandRole, error) {
	if f.byCodeErr != nil {
		return nil, f.byCodeErr
	}
	if v, ok := f.byCode[code]; ok {
		return v, nil
	}
	return nil, apperr.NewAppError(apperr.ErrRoleNotFound, "x", 404)
}
func (f *fakeRoleRepo) ListRoleTemplatesWithPermissions(_ context.Context) ([]*role.RoleTemplate, error) {
	return f.templates, nil
}

type fakeInstrRepo struct {
	get    *instructor.Profile
	getErr error
	upsErr error
	delErr error
}

func (f *fakeInstrRepo) GetByBrandUserID(_ context.Context, _, _ int64) (*instructor.Profile, error) {
	return f.get, f.getErr
}
func (f *fakeInstrRepo) Upsert(_ context.Context, _ int64, in instructor.UpsertInput) (*instructor.Profile, error) {
	if f.upsErr != nil {
		return nil, f.upsErr
	}
	return &instructor.Profile{ID: 7, BrandID: in.BrandID, BrandUserID: in.BrandUserID, DisplayName: in.DisplayName, Status: in.Status}, nil
}
func (f *fakeInstrRepo) Delete(_ context.Context, _, _, _ int64) error { return f.delErr }

// ---- tests ----

// fakePermissionChecker records Require calls and returns configured errors.
type fakePermissionChecker struct {
	requireErrs    map[string]error // by permission code; nil → permit
	requireSeen    []string
	resolveErr     error
	resolveSet     domainrbac.PermissionSet
	resolveScope   domainrbac.DataScope
	invalidatedIDs []int64
	invalidateErr  error
}

func (f *fakePermissionChecker) Require(_ context.Context, _, _ int64, code string) error {
	f.requireSeen = append(f.requireSeen, code)
	if f.requireErrs == nil {
		return nil
	}
	if err, ok := f.requireErrs[code]; ok {
		return err
	}
	return nil
}

func (f *fakePermissionChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return f.resolveSet, f.resolveScope, f.resolveErr
}

func (f *fakePermissionChecker) Invalidate(_ context.Context, brandUserID int64) error {
	f.invalidatedIDs = append(f.invalidatedIDs, brandUserID)
	return f.invalidateErr
}

func (f *fakePermissionChecker) InvalidateMany(_ context.Context, brandUserIDs []int64) error {
	f.invalidatedIDs = append(f.invalidatedIDs, brandUserIDs...)
	return f.invalidateErr
}

func newSvc(sr *fakeStaffRepo, rr *fakeRoleRepo, ir *fakeInstrRepo) *Service {
	if rr == nil {
		rr = &fakeRoleRepo{byCode: map[string]*role.BrandRole{}}
	}
	if ir == nil {
		ir = &fakeInstrRepo{}
	}
	return NewService(sr, rr, ir, nil)
}

func newSvcWithChecker(sr *fakeStaffRepo, rr *fakeRoleRepo, ir *fakeInstrRepo, ch *fakePermissionChecker) *Service {
	if rr == nil {
		rr = &fakeRoleRepo{byCode: map[string]*role.BrandRole{}}
	}
	if ir == nil {
		ir = &fakeInstrRepo{}
	}
	return NewService(sr, rr, ir, ch)
}

func TestCreate_BadPhone(t *testing.T) {
	svc := newSvc(&fakeStaffRepo{}, nil, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Phone: "abc", Name: "x", InitialPassword: "test1234"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc := newSvc(&fakeStaffRepo{}, nil, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Phone: "13900139001", Name: " ", InitialPassword: "test1234"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestCreate_WeakPassword(t *testing.T) {
	svc := newSvc(&fakeStaffRepo{}, nil, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "short"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM (short pw), got %v", err)
	}
	_, err = svc.Create(context.Background(), CreateInput{BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "abcdefgh"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM (no digits), got %v", err)
	}
	_, err = svc.Create(context.Background(), CreateInput{BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "12345678"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM (no letters), got %v", err)
	}
}

func TestCreate_MultiplePrimaryRejected(t *testing.T) {
	svc := newSvc(&fakeStaffRepo{}, nil, nil)
	in := CreateInput{
		BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "test1234",
		LocationAssignments: []staff.LocationAssignmentInput{
			{LocationID: 1, AssignmentType: "manager", IsPrimary: true},
			{LocationID: 2, AssignmentType: "member", IsPrimary: true},
		},
	}
	_, err := svc.Create(context.Background(), in)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestCreate_DuplicateLocationRejected(t *testing.T) {
	svc := newSvc(&fakeStaffRepo{}, nil, nil)
	in := CreateInput{
		BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "test1234",
		LocationAssignments: []staff.LocationAssignmentInput{
			{LocationID: 1, AssignmentType: "manager", IsPrimary: true},
			{LocationID: 1, AssignmentType: "member", IsPrimary: false},
		},
	}
	_, err := svc.Create(context.Background(), in)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrLocationAssignmentInvalid {
		t.Errorf("expected LOCATION_ASSIGNMENT_INVALID, got %v", err)
	}
}

func TestCreate_RejectsBrandOwnerRole(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_owner": {ID: 1, Code: "brand_owner", ScopeType: "brand", Status: "active"},
	}}
	svc := newSvc(&fakeStaffRepo{}, rr, nil)
	in := CreateInput{
		BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "test1234",
		RoleCodes: []string{"brand_owner"},
	}
	_, err := svc.Create(context.Background(), in)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestCreate_QuotaExceededPropagatesFromRepo(t *testing.T) {
	sr := &fakeStaffRepo{createErr: apperr.NewAppError(apperr.ErrQuotaExceeded, "max", 409)}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{}}
	svc := newSvc(sr, rr, nil)
	_, err := svc.Create(context.Background(), CreateInput{
		BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "test1234",
	})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrQuotaExceeded {
		t.Errorf("expected QUOTA_EXCEEDED, got %v", err)
	}
}

func TestCreate_LocationScopeRoleNeedsPrimaryLocation(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"location_manager": {ID: 10, Code: "location_manager", ScopeType: "location", Status: "active"},
	}}
	svc := newSvc(&fakeStaffRepo{}, rr, nil)
	in := CreateInput{
		BrandID: 1, Phone: "13900139001", Name: "x", InitialPassword: "test1234",
		RoleCodes: []string{"location_manager"},
		// 没有 primary location
		LocationAssignments: []staff.LocationAssignmentInput{
			{LocationID: 2, AssignmentType: "manager", IsPrimary: false},
		},
	}
	_, err := svc.Create(context.Background(), in)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM (no primary loc), got %v", err)
	}
}

func TestDelete_OwnerProtected(t *testing.T) {
	sr := &fakeStaffRepo{
		getByID:    &staff.Staff{ID: 1, BrandID: 1, IsOwner: true, Status: staff.StatusActive},
		ownerCount: 1,
	}
	svc := newSvc(sr, nil, nil)
	err := svc.Delete(context.Background(), 1, 0, 1)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrOwnerProtected {
		t.Errorf("expected OWNER_PROTECTED, got %v", err)
	}
}

func TestDelete_NonOwnerOK(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1, IsOwner: false}}
	svc := newSvc(sr, nil, nil)
	if err := svc.Delete(context.Background(), 1, 0, 2); err != nil {
		t.Fatalf("delete should succeed for non-owner, got %v", err)
	}
}

func TestUpdateStatus_OwnerInactiveProtected(t *testing.T) {
	sr := &fakeStaffRepo{
		getByID:    &staff.Staff{ID: 1, BrandID: 1, IsOwner: true},
		ownerCount: 1,
	}
	svc := newSvc(sr, nil, nil)
	_, err := svc.UpdateStatus(context.Background(), 1, 0, 1, "inactive")
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrOwnerProtected {
		t.Errorf("expected OWNER_PROTECTED, got %v", err)
	}
}

func TestReplaceRoleAssignments_RejectsBrandOwner(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_owner": {ID: 1, Code: "brand_owner", ScopeType: "brand", Status: "active"},
	}}
	svc := newSvc(sr, rr, nil)
	_, err := svc.ReplaceRoleAssignments(context.Background(), 1, 0, 2,
		[]staff.RoleAssignmentInput{{RoleCode: "brand_owner"}})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestReplaceRoleAssignments_LocationScopeMissingLocationID(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"location_manager": {ID: 10, Code: "location_manager", ScopeType: "location", Status: "active"},
	}}
	svc := newSvc(sr, rr, nil)
	_, err := svc.ReplaceRoleAssignments(context.Background(), 1, 0, 2,
		[]staff.RoleAssignmentInput{{RoleCode: "location_manager"}})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestReplaceRoleAssignments_BrandScopeWithLocationID(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_admin": {ID: 5, Code: "brand_admin", ScopeType: "brand", Status: "active"},
	}}
	svc := newSvc(sr, rr, nil)
	locID := int64(3)
	_, err := svc.ReplaceRoleAssignments(context.Background(), 1, 0, 2,
		[]staff.RoleAssignmentInput{{RoleCode: "brand_admin", LocationID: &locID}})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestReplaceRoleAssignments_UnknownRole(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{}}
	svc := newSvc(sr, rr, nil)
	_, err := svc.ReplaceRoleAssignments(context.Background(), 1, 0, 2,
		[]staff.RoleAssignmentInput{{RoleCode: "ghost"}})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrRoleNotFound {
		t.Errorf("expected ROLE_NOT_FOUND, got %v", err)
	}
}

func TestReplaceRoleAssignments_InactiveRole(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_admin": {ID: 5, Code: "brand_admin", ScopeType: "brand", Status: "inactive"},
	}}
	svc := newSvc(sr, rr, nil)
	_, err := svc.ReplaceRoleAssignments(context.Background(), 1, 0, 2,
		[]staff.RoleAssignmentInput{{RoleCode: "brand_admin"}})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestReplaceRoleAssignments_EmptyClearsAll(t *testing.T) {
	sr := &fakeStaffRepo{
		getByID:   &staff.Staff{ID: 2, BrandID: 1},
		getAssign: &staff.Staff{ID: 2, BrandID: 1},
	}
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{}}
	svc := newSvc(sr, rr, nil)
	out, err := svc.ReplaceRoleAssignments(context.Background(), 1, 0, 2, nil)
	if err != nil {
		t.Fatalf("empty replace should succeed: %v", err)
	}
	if out == nil {
		t.Errorf("expected staff back")
	}
	if len(sr.roleReplaced) != 0 {
		t.Errorf("expected empty resolved set, got %v", sr.roleReplaced)
	}
}

func TestReplaceLocationAssignments_DuplicateRejected(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	svc := newSvc(sr, nil, nil)
	_, err := svc.ReplaceLocationAssignments(context.Background(), 1, 0, 2,
		[]staff.LocationAssignmentInput{
			{LocationID: 1, AssignmentType: "manager", IsPrimary: true},
			{LocationID: 1, AssignmentType: "member", IsPrimary: false},
		})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrLocationAssignmentInvalid {
		t.Errorf("expected LOCATION_ASSIGNMENT_INVALID, got %v", err)
	}
}

func TestUpsertInstructor_EmptyDisplayName(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	svc := newSvc(sr, nil, &fakeInstrRepo{})
	_, err := svc.UpsertInstructor(context.Background(), 1, 0, 2, instructor.UpsertInput{DisplayName: " "})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestUpsertInstructor_DefaultsActiveStatus(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 2, BrandID: 1}}
	svc := newSvc(sr, nil, &fakeInstrRepo{})
	prof, err := svc.UpsertInstructor(context.Background(), 1, 0, 2, instructor.UpsertInput{DisplayName: "Coach"})
	if err != nil {
		t.Fatal(err)
	}
	if prof.Status != instructor.StatusActive {
		t.Errorf("expected active status default, got %q", prof.Status)
	}
}

func TestGetInstructor_CrossBrandReturnsStaffNotFound(t *testing.T) {
	sr := &fakeStaffRepo{getByIDErr: apperr.NewAppError(apperr.ErrStaffNotFound, "x", 404)}
	svc := newSvc(sr, nil, &fakeInstrRepo{})
	_, err := svc.GetInstructor(context.Background(), 1, 0, 999)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrStaffNotFound {
		t.Errorf("expected STAFF_NOT_FOUND, got %v", err)
	}
}

// ---- Batch 6: RequirePermission gates ----

func deniedErr(code string) error {
	return apperr.NewAppError(apperr.ErrPermissionDenied, "权限不足", 403).
		WithDetails(map[string]any{"required": code, "missing": []string{code}})
}

func TestCreate_PermissionDenied(t *testing.T) {
	// E1
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.create": deniedErr("staff.create")}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, nil, nil, ch)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 18, Phone: "13900139001", Name: "x", InitialPassword: "test1234"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestList_RequiresStaffView(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.view": deniedErr("staff.view")}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, nil, nil, ch)
	_, _, err := svc.List(context.Background(), ListInput{BrandID: 1, ActorID: 18})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestDelete_PermissionDenied(t *testing.T) {
	// E4 — 张三尝试删除自己（没有 staff.delete）
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.delete": deniedErr("staff.delete")}}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 18, BrandID: 1, IsOwner: false}}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	err := svc.Delete(context.Background(), 1, 18, 18)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestReplaceRoleAssignments_PermissionDenied(t *testing.T) {
	// E5 — 没有 staff.assign_role
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.assign_role": deniedErr("staff.assign_role")}}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 18, BrandID: 1}}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	_, err := svc.ReplaceRoleAssignments(context.Background(), 1, 18, 18, nil)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestReplaceLocationAssignments_RequiresAssignLocation(t *testing.T) {
	// E7 implicit — 张三 ReplaceLocationAssignments 需 staff.assign_location
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.assign_location": deniedErr("staff.assign_location")}}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 18, BrandID: 1}}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	_, err := svc.ReplaceLocationAssignments(context.Background(), 1, 18, 18, nil)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestUpsertInstructor_RequiresInstructorEdit(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{"instructor.edit": deniedErr("instructor.edit")}}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 18, BrandID: 1}}
	svc := newSvcWithChecker(sr, nil, &fakeInstrRepo{}, ch)
	_, err := svc.UpsertInstructor(context.Background(), 1, 18, 18, instructor.UpsertInput{DisplayName: "x"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestGetInstructor_RequiresInstructorView(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{"instructor.view": deniedErr("instructor.view")}}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 18, BrandID: 1}}
	svc := newSvcWithChecker(sr, nil, &fakeInstrRepo{}, ch)
	_, err := svc.GetInstructor(context.Background(), 1, 18, 18)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestUpdate_RequiresStaffEdit(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.edit": deniedErr("staff.edit")}}
	sr := &fakeStaffRepo{getAssign: &staff.Staff{ID: 18}}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	_, err := svc.Update(context.Background(), 1, 18, 18, staff.UpdateInput{})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestUpdateStatus_RequiresStaffEdit(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.edit": deniedErr("staff.edit")}}
	sr := &fakeStaffRepo{getAssign: &staff.Staff{ID: 18}}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	_, err := svc.UpdateStatus(context.Background(), 1, 18, 18, "active")
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestListRoles_RequiresStaffView(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{"staff.view": deniedErr("staff.view")}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, nil, nil, ch)
	_, err := svc.ListRoles(context.Background(), 1, 18)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

// Bridge: when checker permits, original behaviour is intact (regression — E33).
func TestCreate_CheckerPermitsThenContinues(t *testing.T) {
	ch := &fakePermissionChecker{}
	sr := &fakeStaffRepo{
		created:   &staff.Staff{ID: 99, BrandID: 1},
		getAssign: &staff.Staff{ID: 99, BrandID: 1},
	}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	if _, err := svc.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 16, Phone: "13900139001", Name: "x", InitialPassword: "test1234"}); err != nil {
		t.Fatalf("expected success when checker permits, got %v", err)
	}
	seen := false
	for _, c := range ch.requireSeen {
		if c == "staff.create" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("expected staff.create check, saw %v", ch.requireSeen)
	}
}

// Silence the unused-var warning from domainrbac import when test list is small.
var _ = domainrbac.DataScope{}
