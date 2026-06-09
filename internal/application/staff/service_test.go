package staff

import (
	"context"
	"testing"

	"github.com/zkw/mini-schedule/backend/internal/domain/instructor"
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
func (f *fakeStaffRepo) Update(_ context.Context, _, _ int64, _ staff.UpdateInput) (*staff.Staff, error) {
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

func newSvc(sr *fakeStaffRepo, rr *fakeRoleRepo, ir *fakeInstrRepo) *Service {
	if rr == nil {
		rr = &fakeRoleRepo{byCode: map[string]*role.BrandRole{}}
	}
	if ir == nil {
		ir = &fakeInstrRepo{}
	}
	return NewService(sr, rr, ir)
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
	_, err := svc.GetInstructor(context.Background(), 1, 999)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrStaffNotFound {
		t.Errorf("expected STAFF_NOT_FOUND, got %v", err)
	}
}
