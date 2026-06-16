package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/locationresource"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

func ptrStr(s string) *string { return &s }
func ptrInt(i int) *int       { return &i }

func TestLocationResource_CreateHappyAndDefaults(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")

	res, err := repo.Create(context.Background(), locationresource.CreateInput{
		BrandID: brandID, ActorID: 1, LocationID: loc, Name: "1号教室", Type: "classroom",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Capacity != 1 {
		t.Fatalf("capacity default should be 1, got %d", res.Capacity)
	}
	if res.Status != locationresource.StatusActive {
		t.Fatalf("status should be active, got %s", res.Status)
	}
	if res.LocationName != "门店1" {
		t.Fatalf("denorm location_name = %q", res.LocationName)
	}
}

func TestLocationResource_CreateDuplicateName(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")

	in := locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: loc, Name: "瑜伽房", Type: "venue", Capacity: 10}
	if _, err := repo.Create(context.Background(), in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(context.Background(), in)
	assertAppCode(t, err, apperr.ErrResourceNameDuplicated)
}

func TestLocationResource_CreateOnInactiveLocationRejected(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	if err := db.Exec(`UPDATE locations SET status='inactive' WHERE id=?`, loc).Error; err != nil {
		t.Fatalf("disable loc: %v", err)
	}
	_, err := repo.Create(context.Background(), locationresource.CreateInput{
		BrandID: brandID, ActorID: 1, LocationID: loc, Name: "x", Type: "other",
	})
	assertAppCode(t, err, apperr.ErrLocationNotFound)
}

func TestLocationResource_ListFiltersAndSoftDelete(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	locA := seedLocation(t, db, brandID, "门店A")
	locB := seedLocation(t, db, brandID, "门店B")

	rA, _ := repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: locA, Name: "A1", Type: "classroom"})
	_, _ = repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: locB, Name: "B1", Type: "venue"})

	// location_id 过滤。
	items, total, err := repo.List(context.Background(), locationresource.ListFilter{BrandID: brandID, LocationID: locA}, 0, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].LocationID != locA {
		t.Fatalf("location filter failed: total=%d", total)
	}

	// 软删后从列表消失。
	if err := repo.Delete(context.Background(), brandID, 1, rA.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, total, _ = repo.List(context.Background(), locationresource.ListFilter{BrandID: brandID, LocationID: locA}, 0, 50)
	if total != 0 {
		t.Fatalf("soft-deleted resource should be excluded, total=%d", total)
	}
	// 同名可再建（unique 仅约束未软删行）。
	if _, err := repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: locA, Name: "A1", Type: "classroom"}); err != nil {
		t.Fatalf("recreate after soft delete: %v", err)
	}
}

func TestLocationResource_ListScopeFilter(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	locA := seedLocation(t, db, brandID, "门店A")
	locB := seedLocation(t, db, brandID, "门店B")
	_, _ = repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: locA, Name: "A1", Type: "classroom"})
	_, _ = repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: locB, Name: "B1", Type: "venue"})

	// scope=[locB] 只返 locB 的资源。
	_, total, _ := repo.List(context.Background(), locationresource.ListFilter{BrandID: brandID, ScopeLocationIDs: []int64{locB}}, 0, 50)
	if total != 1 {
		t.Fatalf("scope filter should keep 1, got %d", total)
	}
	// scope=[] 拒绝所有。
	_, total, _ = repo.List(context.Background(), locationresource.ListFilter{BrandID: brandID, ScopeLocationIDs: []int64{}}, 0, 50)
	if total != 0 {
		t.Fatalf("empty scope should reject all, got %d", total)
	}
}

func TestLocationResource_GetByIDCrossTenantAndSoftDeleted(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	otherBrand, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	res, _ := repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: loc, Name: "R", Type: "other"})

	// 跨租户读不到。
	_, err := repo.GetByID(context.Background(), otherBrand, res.ID)
	assertAppCode(t, err, apperr.ErrResourceNotFound)

	// 软删后读不到。
	_ = repo.Delete(context.Background(), brandID, 1, res.ID)
	_, err = repo.GetByID(context.Background(), brandID, res.ID)
	assertAppCode(t, err, apperr.ErrResourceNotFound)
}

func TestLocationResource_UpdateStatusToggle(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	res, _ := repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: loc, Name: "R", Type: "classroom", Capacity: 5})

	inactive := string(locationresource.StatusInactive)
	updated, err := repo.Update(context.Background(), brandID, 1, res.ID, locationresource.UpdateInput{Status: &inactive, Capacity: ptrInt(8), Name: ptrStr("R2")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Status != locationresource.StatusInactive || updated.Capacity != 8 || updated.Name != "R2" {
		t.Fatalf("update mismatch: %+v", updated)
	}
}

func TestLocationResource_DeleteBlockedByActiveSession(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewLocationResourceRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	instr := seedInstructor(t, db, brandID)
	course := seedPublishedCourseAt(t, db, brandID, loc, "瑜伽")
	res, _ := repo.Create(context.Background(), locationresource.CreateInput{BrandID: brandID, ActorID: 1, LocationID: loc, Name: "教室", Type: "classroom", Capacity: 6})

	// 直接 INSERT 一节 scheduled 场次引用该资源。
	start := time.Now().UTC().Add(48 * time.Hour)
	if err := db.Exec(
		`INSERT INTO class_sessions (brand_id, location_id, location_resource_id, course_id, instructor_profile_id, starts_at, ends_at, capacity, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'scheduled')`,
		brandID, loc, res.ID, course, instr, start, start.Add(time.Hour), 6,
	).Error; err != nil {
		t.Fatalf("insert session: %v", err)
	}

	err := repo.Delete(context.Background(), brandID, 1, res.ID)
	assertAppCode(t, err, apperr.ErrResourceInUse)
}

// assertAppCode 断言 err 是带指定 code 的 AppError。
func assertAppCode(t *testing.T, err error, code apperr.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	ae, ok := err.(*apperr.AppError)
	if !ok {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if ae.Code != code {
		t.Fatalf("expected code %s, got %s (%v)", code, ae.Code, err)
	}
}
