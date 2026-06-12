package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
)

// seedLocation inserts an active location for the given brand and returns its id.
func seedLocation(t *testing.T, db *gorm.DB, brandID int64, name string) int64 {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO locations (brand_id, name, status) VALUES (?, ?, 'active')`,
		brandID, name,
	).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	var id int64
	if err := db.Raw(
		`SELECT id FROM locations WHERE brand_id = ? AND name = ? ORDER BY id DESC LIMIT 1`,
		brandID, name,
	).Scan(&id).Error; err != nil {
		t.Fatalf("read location id: %v", err)
	}
	return id
}

// seedBrandUser inserts an active brand_user and returns its id.
func seedBrandUser(t *testing.T, db *gorm.DB, brandID int64) int64 {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO brand_users (brand_id, phone, password_hash, name, status)
		 VALUES (?, ?, 'h', '员工', 'active')`,
		brandID, fmt.Sprintf("3%010d", time.Now().UnixNano()%1e10),
	).Error; err != nil {
		t.Fatalf("seed brand_user: %v", err)
	}
	var id int64
	if err := db.Raw(`SELECT id FROM brand_users WHERE brand_id = ? ORDER BY id DESC LIMIT 1`, brandID).
		Scan(&id).Error; err != nil {
		t.Fatalf("read brand_user id: %v", err)
	}
	return id
}

// TestCountActiveReferences_StaffAndRole verifies the delete-guard counter (BE-4):
// staff_location_assignments + brand_user_role_assignments are summed, and a
// reference belonging to a DIFFERENT brand's location id is NOT counted.
func TestCountActiveReferences_StaffAndRole(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationRepository(db, &commercial.SubscriptionGuard{}).(*locationRepository)

	brandID, ownerRoleID := seedBrandWithSystemRoles(t, db)
	locID := seedLocation(t, db, brandID, "总店")
	userID := seedBrandUser(t, db, brandID)

	// No references yet.
	n, err := repo.CountActiveReferences(context.Background(), brandID, locID)
	if err != nil || n != 0 {
		t.Fatalf("count (no refs) = %d err = %v, want 0", n, err)
	}

	// One active staff_location_assignment → count == 1.
	if err := db.Exec(
		`INSERT INTO staff_location_assignments (brand_id, brand_user_id, location_id, assignment_type, is_primary, status)
		 VALUES (?, ?, ?, 'member', TRUE, 'active')`,
		brandID, userID, locID,
	).Error; err != nil {
		t.Fatalf("seed staff_location_assignment: %v", err)
	}
	n, err = repo.CountActiveReferences(context.Background(), brandID, locID)
	if err != nil || n != 1 {
		t.Fatalf("count (1 staff) = %d err = %v, want 1", n, err)
	}

	// Add an active location-scoped brand_user_role_assignment → count == 2.
	if err := db.Exec(
		`INSERT INTO brand_user_role_assignments (brand_id, brand_user_id, role_id, location_id, data_scope, status)
		 VALUES (?, ?, ?, ?, 'role_default', 'active')`,
		brandID, userID, ownerRoleID, locID,
	).Error; err != nil {
		t.Fatalf("seed brand_user_role_assignment: %v", err)
	}
	n, err = repo.CountActiveReferences(context.Background(), brandID, locID)
	if err != nil || n != 2 {
		t.Fatalf("count (staff+role) = %d err = %v, want 2", n, err)
	}

	// brand_id isolation: a reference for a DIFFERENT brand whose assignment row
	// carries the SAME location_id must NOT be counted against this brand.
	otherBrandID, otherRoleID := seedBrandWithSystemRoles(t, db)
	otherUserID := seedBrandUser(t, db, otherBrandID)
	otherLocID := seedLocation(t, db, otherBrandID, "他牌门店")
	if err := db.Exec(
		`INSERT INTO staff_location_assignments (brand_id, brand_user_id, location_id, assignment_type, is_primary, status)
		 VALUES (?, ?, ?, 'member', TRUE, 'active')`,
		otherBrandID, otherUserID, otherLocID,
	).Error; err != nil {
		t.Fatalf("seed other-brand staff assignment: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO brand_user_role_assignments (brand_id, brand_user_id, role_id, location_id, data_scope, status)
		 VALUES (?, ?, ?, ?, 'role_default', 'active')`,
		otherBrandID, otherUserID, otherRoleID, otherLocID,
	).Error; err != nil {
		t.Fatalf("seed other-brand role assignment: %v", err)
	}
	// Counting our brand's location is unaffected by the other brand's rows.
	n, err = repo.CountActiveReferences(context.Background(), brandID, locID)
	if err != nil || n != 2 {
		t.Fatalf("count after other-brand rows = %d err = %v, want 2 (brand_id isolation)", n, err)
	}

	// Sanity: querying our brand against the OTHER brand's location id counts 0.
	n, err = repo.CountActiveReferences(context.Background(), brandID, otherLocID)
	if err != nil || n != 0 {
		t.Fatalf("count (cross-brand location id) = %d err = %v, want 0", n, err)
	}

	// And inactive rows are excluded (filter status='active').
	if err := db.Exec(
		`UPDATE staff_location_assignments SET status = 'inactive' WHERE brand_id = ? AND location_id = ?`,
		brandID, locID,
	).Error; err != nil {
		t.Fatalf("deactivate staff assignment: %v", err)
	}
	n, err = repo.CountActiveReferences(context.Background(), brandID, locID)
	if err != nil || n != 1 {
		t.Fatalf("count after deactivating staff = %d err = %v, want 1 (only active role left)", n, err)
	}
}
