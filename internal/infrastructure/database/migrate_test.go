package database

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/zkw/mini-schedule/backend/migrations"
)

// migrationTestDSN resolves the admin DSN used to create/drop a throwaway test
// database. Order: MINI_SCHEDULE_TEST_DATABASE_URL env → local default. If no
// Postgres is reachable the test skips (CI without a DB must not fail).
func migrationTestDSN(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("MINI_SCHEDULE_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://liushan@127.0.0.1:5432/postgres?sslmode=disable"
}

// TestMigrationsUpToLatest brings an empty database up to the newest migration
// (000006 at time of writing) and asserts the Batch 7 `role.manage` permission
// + its brand_owner/brand_admin template mappings landed (E9).
func TestMigrationsUpToLatest(t *testing.T) {
	adminDSN := migrationTestDSN(t)

	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Skipf("cannot open admin DSN, skipping migration test: %v", err)
	}
	defer admin.Close()

	admin.SetConnMaxLifetime(5 * time.Second)
	if err := admin.Ping(); err != nil {
		t.Skipf("no Postgres reachable, skipping migration test: %v", err)
	}

	dbName := fmt.Sprintf("ms_migtest_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		// Terminate stray connections then drop.
		_, _ = admin.Exec(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
			dbName,
		)
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
			t.Logf("drop test database %s: %v", dbName, err)
		}
	})

	testDSN := fmt.Sprintf("postgres://liushan@127.0.0.1:5432/%s?sslmode=disable", dbName)
	if env := os.Getenv("MINI_SCHEDULE_TEST_DATABASE_URL"); env != "" {
		// When an explicit admin URL is given, derive the per-test DB URL by
		// swapping the database segment is non-trivial; require a template var.
		if tmpl := os.Getenv("MINI_SCHEDULE_TEST_DATABASE_URL_TEMPLATE"); tmpl != "" {
			testDSN = fmt.Sprintf(tmpl, dbName)
		}
	}

	if err := RunMigrationsUp(testDSN, migrations.FS, nil); err != nil {
		t.Fatalf("RunMigrationsUp to latest failed: %v", err)
	}

	conn, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Fatalf("open migrated test DB: %v", err)
	}
	defer conn.Close()

	// E9: role.manage permission exists.
	var permCount int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM permissions WHERE code = 'role.manage' AND domain = 'role' AND action = 'manage'",
	).Scan(&permCount); err != nil {
		t.Fatalf("query role.manage permission: %v", err)
	}
	if permCount != 1 {
		t.Fatalf("role.manage permission count = %d, want 1", permCount)
	}

	// E9: brand_owner + brand_admin templates both mapped to role.manage.
	var mapCount int
	if err := conn.QueryRow(`
		SELECT COUNT(*)
		FROM role_template_permissions rtp
		JOIN role_templates rt ON rt.id = rtp.template_id
		JOIN permissions p ON p.id = rtp.permission_id
		WHERE p.code = 'role.manage' AND rt.code IN ('brand_owner', 'brand_admin')
	`).Scan(&mapCount); err != nil {
		t.Fatalf("query role.manage template mappings: %v", err)
	}
	if mapCount != 2 {
		t.Fatalf("role.manage template mapping count = %d, want 2 (brand_owner + brand_admin)", mapCount)
	}
}
