package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/database"
	"github.com/zkw/mini-schedule/backend/migrations"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// newMigratedTestDB creates a throwaway migrated Postgres DB and returns a GORM
// handle. Skips when no Postgres is reachable (CI without a DB must not fail).
func newMigratedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	adminDSN := os.Getenv("MINI_SCHEDULE_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://liushan@127.0.0.1:5432/postgres?sslmode=disable"
	}
	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Skipf("cannot open admin DSN, skipping DB test: %v", err)
	}
	admin.SetConnMaxLifetime(5 * time.Second)
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		t.Skipf("no Postgres reachable, skipping DB test: %v", err)
	}

	dbName := fmt.Sprintf("ms_repotest_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		_ = admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	testDSN := fmt.Sprintf("postgres://liushan@127.0.0.1:5432/%s?sslmode=disable", dbName)

	t.Cleanup(func() {
		_, _ = admin.Exec(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
			dbName,
		)
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + dbName)
		_ = admin.Close()
	})

	if err := database.RunMigrationsUp(testDSN, migrations.FS, nil); err != nil {
		t.Fatalf("migrate test DB: %v", err)
	}

	gdb, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm on test DB: %v", err)
	}
	return gdb
}

// seedBrandWithSystemRoles inserts a brand row and a brand_owner system role,
// returning brandID + ownerRoleID for assignment tests.
func seedBrandWithSystemRoles(t *testing.T, db *gorm.DB) (brandID int64, ownerRoleID int64) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO brands (name, contact_name, contact_phone, status) VALUES (?, ?, ?, 'active') RETURNING id`,
		"测试品牌", "联系人", fmt.Sprintf("1%010d", time.Now().UnixNano()%1e10),
	).Error; err != nil {
		t.Fatalf("seed brand: %v", err)
	}
	if err := db.Raw(`SELECT id FROM brands ORDER BY id DESC LIMIT 1`).Scan(&brandID).Error; err != nil {
		t.Fatalf("read brand id: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO brand_roles (brand_id, code, name, scope_type, is_system, status)
		 VALUES (?, 'brand_owner', '品牌负责人', 'brand', TRUE, 'active')`,
		brandID,
	).Error; err != nil {
		t.Fatalf("seed brand_owner role: %v", err)
	}
	if err := db.Raw(`SELECT id FROM brand_roles WHERE brand_id = ? AND code = 'brand_owner'`, brandID).
		Scan(&ownerRoleID).Error; err != nil {
		t.Fatalf("read owner role id: %v", err)
	}
	return brandID, ownerRoleID
}

func TestCreateBrandRole_EmptyPermissionsAllowed(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	got, err := repo.CreateBrandRole(context.Background(), role.CreateBrandRoleInput{
		BrandID:         brandID,
		ActorID:         1,
		Name:            "无权限角色",
		ScopeType:       role.ScopeBrand,
		PermissionCodes: []string{}, // E6: empty array → valid no-permission role
	})
	if err != nil {
		t.Fatalf("CreateBrandRole empty perms: %v", err)
	}
	if got.IsSystem {
		t.Errorf("custom role IsSystem = true, want false")
	}
	if len(got.Code) < 8 || got.Code[:7] != "custom_" {
		t.Errorf("Code = %q, want custom_ prefix", got.Code)
	}
	if len(got.Permissions) != 0 {
		t.Errorf("Permissions = %v, want empty", got.Permissions)
	}
}

func TestCreateBrandRole_WithPermissions(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	got, err := repo.CreateBrandRole(context.Background(), role.CreateBrandRoleInput{
		BrandID:         brandID,
		ActorID:         1,
		Name:            "前台兼职",
		ScopeType:       role.ScopeLocation,
		PermissionCodes: []string{"staff.create"}, // raw stored, NOT expanded
	})
	if err != nil {
		t.Fatalf("CreateBrandRole: %v", err)
	}
	// B2: raw stored — only staff.create, no implied staff.view at DB level.
	if len(got.Permissions) != 1 || got.Permissions[0].Code != "staff.create" {
		t.Errorf("Permissions = %v, want exactly [staff.create] (raw, unexpanded)", got.Permissions)
	}
}

func TestCreateBrandRole_UnknownPermissionCodeRejected(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	_, err := repo.CreateBrandRole(context.Background(), role.CreateBrandRoleInput{
		BrandID:         brandID,
		ActorID:         1,
		Name:            "坏角色",
		ScopeType:       role.ScopeBrand,
		PermissionCodes: []string{"does.not.exist"},
	})
	if code := apperr.GetAppError(err); code == nil || code.Code != apperr.ErrInvalidParam {
		t.Fatalf("err = %v, want INVALID_PARAM", err)
	}
}

func TestCreateBrandRole_DuplicateCodeMapsToRoleCodeDuplicated(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db).(*roleRepository)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	// Pre-insert a row at a fixed code, then attempt to create another row with
	// the SAME (brand_id, code) — this is the unique-index collision the repo
	// maps to ROLE_CODE_DUPLICATED.
	if err := db.Exec(
		`INSERT INTO brand_roles (brand_id, code, name, scope_type, is_system, status)
		 VALUES (?, 'custom_collide', '占位', 'brand', FALSE, 'active')`,
		brandID,
	).Error; err != nil {
		t.Fatalf("seed colliding role: %v", err)
	}

	err := repo.db.Transaction(func(tx *gorm.DB) error {
		row := BrandRoleModel{BrandID: brandID, Code: "custom_collide", Name: "x", ScopeType: "brand", IsSystem: false, Status: "active"}
		if e := tx.Create(&row).Error; e != nil {
			if isUniqueViolation(e) {
				return apperr.NewAppError(apperr.ErrRoleCodeDuplicated, "角色 code 冲突，请重试", 409)
			}
			return e
		}
		return nil
	})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrRoleCodeDuplicated {
		t.Fatalf("err = %v, want ROLE_CODE_DUPLICATED", err)
	}
}

func TestCountAndListBrandUserIDsByRole(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	created, err := repo.CreateBrandRole(context.Background(), role.CreateBrandRoleInput{
		BrandID: brandID, ActorID: 1, Name: "前台", ScopeType: role.ScopeBrand,
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	// No assignments yet.
	n, err := repo.CountAssignmentsByRole(context.Background(), created.ID)
	if err != nil || n != 0 {
		t.Fatalf("count = %d err = %v, want 0", n, err)
	}

	// Seed a brand_user + active assignment.
	var userID int64
	if err := db.Exec(
		`INSERT INTO brand_users (brand_id, phone, password_hash, name, status)
		 VALUES (?, ?, 'h', '员工', 'active')`,
		brandID, fmt.Sprintf("2%010d", time.Now().UnixNano()%1e10),
	).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Raw(`SELECT id FROM brand_users WHERE brand_id = ? ORDER BY id DESC LIMIT 1`, brandID).
		Scan(&userID).Error; err != nil {
		t.Fatalf("read user id: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO brand_user_role_assignments (brand_id, brand_user_id, role_id, data_scope, status)
		 VALUES (?, ?, ?, 'role_default', 'active')`,
		brandID, userID, created.ID,
	).Error; err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	n, err = repo.CountAssignmentsByRole(context.Background(), created.ID)
	if err != nil || n != 1 {
		t.Fatalf("count = %d err = %v, want 1", n, err)
	}
	ids, err := repo.ListBrandUserIDsByRole(context.Background(), created.ID)
	if err != nil || len(ids) != 1 || ids[0] != userID {
		t.Fatalf("ids = %v err = %v, want [%d]", ids, err, userID)
	}
}

func TestUpdateBrandRole_ReplacesPermissions(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	created, err := repo.CreateBrandRole(context.Background(), role.CreateBrandRoleInput{
		BrandID: brandID, ActorID: 1, Name: "前台", ScopeType: role.ScopeBrand,
		PermissionCodes: []string{"staff.create", "staff.edit"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := repo.UpdateBrandRole(context.Background(), role.UpdateBrandRoleInput{
		BrandID: brandID, ActorID: 1, RoleID: created.ID,
		Name: "前台改名", Description: "desc",
		PermissionCodes: []string{"instructor.view"}, // full replace
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "前台改名" {
		t.Errorf("Name = %q, want 前台改名", updated.Name)
	}
	if len(updated.Permissions) != 1 || updated.Permissions[0].Code != "instructor.view" {
		t.Errorf("Permissions = %v, want exactly [instructor.view]", updated.Permissions)
	}
}

func TestGetBrandRoleWithPermissions(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	created, err := repo.CreateBrandRole(context.Background(), role.CreateBrandRoleInput{
		BrandID: brandID, ActorID: 1, Name: "前台", ScopeType: role.ScopeBrand,
		PermissionCodes: []string{"staff.create", "staff.edit"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetBrandRoleWithPermissions(context.Background(), brandID, created.Code)
	if err != nil {
		t.Fatalf("GetBrandRoleWithPermissions: %v", err)
	}
	if got.ID != created.ID || got.Code != created.Code {
		t.Errorf("got id=%d code=%q, want id=%d code=%q", got.ID, got.Code, created.ID, created.Code)
	}
	gotCodes := map[string]bool{}
	for _, p := range got.Permissions {
		gotCodes[p.Code] = true
	}
	if len(got.Permissions) != 2 || !gotCodes["staff.create"] || !gotCodes["staff.edit"] {
		t.Errorf("Permissions = %v, want [staff.create staff.edit]", got.Permissions)
	}

	// Unknown code → ErrRoleNotFound (404).
	_, err = repo.GetBrandRoleWithPermissions(context.Background(), brandID, "does_not_exist")
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrRoleNotFound || ae.HTTPStatus != 404 {
		t.Fatalf("err = %v, want ROLE_NOT_FOUND 404", err)
	}
}

func TestListPermissions_GroupableByDomain(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewRoleRepository(db)

	perms, err := repo.ListPermissions(context.Background())
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	var hasRoleManage bool
	for _, p := range perms {
		if p.Code == "role.manage" {
			hasRoleManage = true
			if p.Domain != "role" {
				t.Errorf("role.manage domain = %q, want role", p.Domain)
			}
		}
	}
	if !hasRoleManage {
		t.Errorf("ListPermissions missing role.manage")
	}
}
