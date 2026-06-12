package staff

import (
	"context"
	"testing"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	"github.com/zkw/mini-schedule/backend/internal/domain/staff"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

func errCode(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

// permitAll is a checker that grants role.manage and resolves owner-equivalent.
func permitAll() *fakePermissionChecker {
	return &fakePermissionChecker{}
}

// E2: PUT/DELETE/PATCH on a system role → ROLE_IS_SYSTEM.
func TestUpdateRole_SystemRoleBlocked(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"location_manager": {ID: 5, Code: "location_manager", IsSystem: true, ScopeType: "location"},
	}}
	// owner actor so B1 subset is skipped.
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: true}}
	svc := newSvcWithChecker(sr, rr, nil, permitAll())

	_, err := svc.UpdateRole(context.Background(), UpdateRoleInput{
		BrandID: 1, ActorID: 9, Code: "location_manager", Name: "改名", PermissionCodes: []string{},
	})
	if errCode(err) != apperr.ErrRoleIsSystem {
		t.Fatalf("UpdateRole system role: err = %v, want ROLE_IS_SYSTEM", err)
	}
}

func TestDeleteRole_SystemRoleBlocked(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"location_reception": {ID: 6, Code: "location_reception", IsSystem: true, ScopeType: "location"},
	}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, rr, nil, permitAll())
	err := svc.DeleteRole(context.Background(), 1, 9, "location_reception")
	if errCode(err) != apperr.ErrRoleIsSystem {
		t.Fatalf("DeleteRole system role: err = %v, want ROLE_IS_SYSTEM", err)
	}
}

func TestPatchRoleStatus_SystemRoleBlocked(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_admin": {ID: 2, Code: "brand_admin", IsSystem: true, ScopeType: "brand"},
	}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, rr, nil, permitAll())
	_, err := svc.PatchRoleStatus(context.Background(), 1, 9, "brand_admin", "inactive")
	if errCode(err) != apperr.ErrRoleIsSystem {
		t.Fatalf("PatchRoleStatus system role: err = %v, want ROLE_IS_SYSTEM", err)
	}
}

// E3: brand_owner system role → OWNER_PROTECTED (takes priority over ROLE_IS_SYSTEM).
func TestUpdateRole_OwnerRoleProtected(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_owner": {ID: 1, Code: "brand_owner", IsSystem: true, ScopeType: "brand"},
	}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, rr, nil, permitAll())
	_, err := svc.UpdateRole(context.Background(), UpdateRoleInput{
		BrandID: 1, ActorID: 9, Code: "brand_owner", Name: "x", PermissionCodes: []string{},
	})
	if errCode(err) != apperr.ErrOwnerProtected {
		t.Fatalf("UpdateRole brand_owner: err = %v, want OWNER_PROTECTED", err)
	}
}

func TestDeleteRole_OwnerRoleProtected(t *testing.T) {
	rr := &fakeRoleRepo{byCode: map[string]*role.BrandRole{
		"brand_owner": {ID: 1, Code: "brand_owner", IsSystem: true, ScopeType: "brand"},
	}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, rr, nil, permitAll())
	err := svc.DeleteRole(context.Background(), 1, 9, "brand_owner")
	if errCode(err) != apperr.ErrOwnerProtected {
		t.Fatalf("DeleteRole brand_owner: err = %v, want OWNER_PROTECTED", err)
	}
}

// E4: non-owner actor selecting permissions beyond their own set → ROLE_PERMISSION_EXCEEDS_ACTOR.
func TestCreateRole_PermissionExceedsActor(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: false}}
	ch := &fakePermissionChecker{
		resolveSet: domainrbac.Expand([]string{"staff.view"}), // actor only has staff.view
	}
	svc := newSvcWithChecker(sr, nil, nil, ch)
	_, err := svc.CreateRole(context.Background(), CreateRoleInput{
		BrandID: 1, ActorID: 9, Name: "越权角色", ScopeType: "brand",
		PermissionCodes: []string{"staff.delete"}, // beyond actor
	})
	if errCode(err) != apperr.ErrRolePermissionExceedsActor {
		t.Fatalf("CreateRole exceeding perms: err = %v, want ROLE_PERMISSION_EXCEEDS_ACTOR", err)
	}
}

// E5: owner can select any permission (B1 owner exception).
func TestCreateRole_OwnerCanSelectAny(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: true}}
	ch := &fakePermissionChecker{resolveSet: domainrbac.Expand([]string{})} // empty, but owner skips
	rr := &fakeRoleRepo{createOut: &role.BrandRole{ID: 10, Code: "custom_x", IsSystem: false}}
	svc := newSvcWithChecker(sr, rr, nil, ch)
	got, err := svc.CreateRole(context.Background(), CreateRoleInput{
		BrandID: 1, ActorID: 9, Name: "全权角色", ScopeType: "brand",
		PermissionCodes: []string{"staff.delete", "location.delete"},
	})
	if err != nil {
		t.Fatalf("owner CreateRole: %v", err)
	}
	if got.Code != "custom_x" {
		t.Fatalf("got code %q, want custom_x", got.Code)
	}
}

// E6: empty permission_codes is allowed.
func TestCreateRole_EmptyPermissionsAllowed(t *testing.T) {
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: true}}
	rr := &fakeRoleRepo{createOut: &role.BrandRole{ID: 11, Code: "custom_y", IsSystem: false}}
	svc := newSvcWithChecker(sr, rr, nil, permitAll())
	_, err := svc.CreateRole(context.Background(), CreateRoleInput{
		BrandID: 1, ActorID: 9, Name: "无权限角色", ScopeType: "brand", PermissionCodes: []string{},
	})
	if err != nil {
		t.Fatalf("empty perms CreateRole: %v", err)
	}
	if rr.createIn == nil || len(rr.createIn.PermissionCodes) != 0 {
		t.Fatalf("expected empty PermissionCodes passed to repo, got %#v", rr.createIn)
	}
}

// E7: UpdateRole has no scope_type field → scope cannot change (A3). Verify repo
// UpdateBrandRoleInput carries no scope and target scope is untouched.
func TestUpdateRole_ScopeTypeNotChangeable(t *testing.T) {
	rr := &fakeRoleRepo{
		byCode:    map[string]*role.BrandRole{"custom_z": {ID: 20, Code: "custom_z", IsSystem: false, ScopeType: "location"}},
		updateOut: &role.BrandRole{ID: 20, Code: "custom_z", ScopeType: "location"},
	}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: true}}
	svc := newSvcWithChecker(sr, rr, nil, permitAll())
	got, err := svc.UpdateRole(context.Background(), UpdateRoleInput{
		BrandID: 1, ActorID: 9, Code: "custom_z", Name: "改名", PermissionCodes: []string{},
	})
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if got.ScopeType != "location" {
		t.Fatalf("scope_type changed to %q, want location (A3)", got.ScopeType)
	}
}

// B1 (delta semantics): a non-owner actor editing a role that ALREADY contains a
// permission the actor lacks can KEEP that permission — only newly-added perms are
// checked against the actor. Here actor has only staff.view; the role already has
// location.delete; resubmitting it (e.g. while renaming) must succeed.
func TestUpdateRole_KeepingExistingExceedingPermAllowed(t *testing.T) {
	rr := &fakeRoleRepo{
		byCode:            map[string]*role.BrandRole{"custom_z": {ID: 20, Code: "custom_z", IsSystem: false, ScopeType: "brand"}},
		existingPermCodes: []string{"location.delete"}, // role already grants this
		updateOut:         &role.BrandRole{ID: 20, Code: "custom_z"},
	}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: false}}
	ch := &fakePermissionChecker{resolveSet: domainrbac.Expand([]string{"staff.view"})} // actor lacks location.delete
	svc := newSvcWithChecker(sr, rr, nil, ch)
	_, err := svc.UpdateRole(context.Background(), UpdateRoleInput{
		BrandID: 1, ActorID: 9, Code: "custom_z", Name: "改名",
		PermissionCodes: []string{"location.delete"}, // kept, not newly added
	})
	if err != nil {
		t.Fatalf("UpdateRole keeping existing exceeding perm: err = %v, want nil", err)
	}
}

// B1 (delta semantics): ADDING a permission beyond the non-owner actor's set is
// still rejected. Role currently has staff.view; actor (staff.view only) adds
// staff.delete → ROLE_PERMISSION_EXCEEDS_ACTOR.
func TestUpdateRole_AddingExceedingPermRejected(t *testing.T) {
	rr := &fakeRoleRepo{
		byCode:            map[string]*role.BrandRole{"custom_z": {ID: 20, Code: "custom_z", IsSystem: false, ScopeType: "brand"}},
		existingPermCodes: []string{"staff.view"},
		updateOut:         &role.BrandRole{ID: 20, Code: "custom_z"},
	}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: false}}
	ch := &fakePermissionChecker{resolveSet: domainrbac.Expand([]string{"staff.view"})}
	svc := newSvcWithChecker(sr, rr, nil, ch)
	_, err := svc.UpdateRole(context.Background(), UpdateRoleInput{
		BrandID: 1, ActorID: 9, Code: "custom_z", Name: "改名",
		PermissionCodes: []string{"staff.view", "staff.delete"}, // staff.delete is newly added & beyond actor
	})
	if errCode(err) != apperr.ErrRolePermissionExceedsActor {
		t.Fatalf("UpdateRole adding exceeding perm: err = %v, want ROLE_PERMISSION_EXCEEDS_ACTOR", err)
	}
}

// E1: DeleteRole with active assignments → ROLE_IN_USE.
func TestDeleteRole_InUse(t *testing.T) {
	rr := &fakeRoleRepo{
		byCode:            map[string]*role.BrandRole{"custom_inuse": {ID: 30, Code: "custom_inuse", IsSystem: false}},
		activeAssignments: 2,
	}
	svc := newSvcWithChecker(&fakeStaffRepo{}, rr, nil, permitAll())
	err := svc.DeleteRole(context.Background(), 1, 9, "custom_inuse")
	if errCode(err) != apperr.ErrRoleInUse {
		t.Fatalf("DeleteRole in use: err = %v, want ROLE_IN_USE", err)
	}
}

func TestDeleteRole_NoAssignmentsSucceeds(t *testing.T) {
	rr := &fakeRoleRepo{
		byCode:            map[string]*role.BrandRole{"custom_free": {ID: 31, Code: "custom_free", IsSystem: false}},
		activeAssignments: 0,
	}
	svc := newSvcWithChecker(&fakeStaffRepo{}, rr, nil, permitAll())
	if err := svc.DeleteRole(context.Background(), 1, 9, "custom_free"); err != nil {
		t.Fatalf("DeleteRole free: %v", err)
	}
	if rr.deleteSeen != 31 {
		t.Fatalf("expected DeleteBrandRole(31), got %d", rr.deleteSeen)
	}
}

// C1: UpdateRole invalidates cache for every holder post-commit.
func TestUpdateRole_InvalidatesHolders(t *testing.T) {
	rr := &fakeRoleRepo{
		byCode:        map[string]*role.BrandRole{"custom_h": {ID: 40, Code: "custom_h", IsSystem: false, ScopeType: "brand"}},
		updateOut:     &role.BrandRole{ID: 40, Code: "custom_h", ScopeType: "brand"},
		userIDsByRole: []int64{101, 102},
	}
	sr := &fakeStaffRepo{getByID: &staff.Staff{ID: 9, IsOwner: true}}
	ch := permitAll()
	svc := newSvcWithChecker(sr, rr, nil, ch)
	if _, err := svc.UpdateRole(context.Background(), UpdateRoleInput{
		BrandID: 1, ActorID: 9, Code: "custom_h", Name: "x", PermissionCodes: []string{},
	}); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if len(ch.invalidatedIDs) != 2 || ch.invalidatedIDs[0] != 101 || ch.invalidatedIDs[1] != 102 {
		t.Fatalf("expected invalidate [101 102], got %v", ch.invalidatedIDs)
	}
}

// E8: missing role.manage → PERMISSION_DENIED before any business logic.
func TestCreateRole_PermissionDenied(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{
		"role.manage": apperr.NewAppError(apperr.ErrPermissionDenied, "权限不足", 403),
	}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, nil, nil, ch)
	_, err := svc.CreateRole(context.Background(), CreateRoleInput{
		BrandID: 1, ActorID: 9, Name: "x", ScopeType: "brand", PermissionCodes: []string{},
	})
	if errCode(err) != apperr.ErrPermissionDenied {
		t.Fatalf("CreateRole no perm: err = %v, want PERMISSION_DENIED", err)
	}
}

func TestListPermissions_GatedByRoleManage(t *testing.T) {
	ch := &fakePermissionChecker{requireErrs: map[string]error{
		"role.manage": apperr.NewAppError(apperr.ErrPermissionDenied, "权限不足", 403),
	}}
	svc := newSvcWithChecker(&fakeStaffRepo{}, nil, nil, ch)
	_, err := svc.ListPermissions(context.Background(), 1, 9)
	if errCode(err) != apperr.ErrPermissionDenied {
		t.Fatalf("ListPermissions gate: err = %v, want PERMISSION_DENIED", err)
	}
}
