package persistence

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/coursetemplate"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// seedCategory inserts an active course category and returns its id.
func seedCategory(t *testing.T, db *gorm.DB, brandID int64, name string) int64 {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO course_categories (brand_id, name, status) VALUES (?, ?, 'active')`,
		brandID, name,
	).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	var id int64
	if err := db.Raw(`SELECT id FROM course_categories WHERE brand_id = ? AND name = ?`, brandID, name).
		Scan(&id).Error; err != nil {
		t.Fatalf("read category id: %v", err)
	}
	return id
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func TestCourseTemplate_CreateGetWithCategoriesAndLocations(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc1 := seedLocation(t, db, brandID, "门店1")
	loc2 := seedLocation(t, db, brandID, "门店2")
	cat := seedCategory(t, db, brandID, "团课")

	got, err := repo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "晨间瑜伽", DurationMin: 60, DefaultCapacity: 8,
		CategoryIDs: []int64{cat}, LocationIDs: []int64{loc1, loc2},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Status != coursetemplate.StatusDraft {
		t.Errorf("status = %s, want draft", got.Status)
	}
	if len(got.Categories) != 1 || got.Categories[0].ID != cat {
		t.Errorf("categories = %+v, want [%d]", got.Categories, cat)
	}
	if got.AvailableLocationCount != 2 {
		t.Errorf("available count = %d, want 2", got.AvailableLocationCount)
	}
	if len(got.AvailableLocationIDs) != 2 {
		t.Errorf("available ids = %v, want 2", got.AvailableLocationIDs)
	}
}

func TestCourseTemplate_CreateEmptyLocationsDefaultsToAllActive(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedLocation(t, db, brandID, "门店1")
	seedLocation(t, db, brandID, "门店2")

	got, err := repo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "默认全选", DurationMin: 45, DefaultCapacity: 5,
		LocationIDs: nil,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.AvailableLocationCount != 2 {
		t.Errorf("empty location_ids should default to all active (2), got %d", got.AvailableLocationCount)
	}
}

func TestCourseTemplate_CreateInvalidCategory(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedLocation(t, db, brandID, "门店1")

	_, err := repo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "坏分类", DurationMin: 60, DefaultCapacity: 8,
		CategoryIDs: []int64{99999},
	})
	if codeOf(err) != apperr.ErrCategoryNotFound {
		t.Fatalf("want CATEGORY_NOT_FOUND, got %v", err)
	}
}

func TestCourseTemplate_SoftDeleteExcludedFromList(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedLocation(t, db, brandID, "门店1")

	c, err := repo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "待删", DurationMin: 60, DefaultCapacity: 8,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.SoftDelete(context.Background(), brandID, 1, c.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	items, total, err := repo.List(context.Background(), coursetemplate.ListFilter{BrandID: brandID}, 0, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("soft-deleted course should be excluded, got total=%d", total)
	}
	if _, err := repo.GetByID(context.Background(), brandID, c.ID); codeOf(err) != apperr.ErrCourseNotFound {
		t.Errorf("get soft-deleted = %v, want COURSE_NOT_FOUND", err)
	}
}

func TestCourseTemplate_UpdateStatusSetsPublishedAt(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewCourseTemplateRepository(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedLocation(t, db, brandID, "门店1")

	c, _ := repo.Create(context.Background(), coursetemplate.CreateInput{
		BrandID: brandID, ActorID: 1, Title: "发布", DurationMin: 60, DefaultCapacity: 8,
	})
	got, err := repo.UpdateStatus(context.Background(), brandID, 1, c.ID, coursetemplate.StatusPublished)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got.Status != coursetemplate.StatusPublished || got.PublishedAt == nil {
		t.Errorf("published_at should be set on first publish, got status=%s published_at=%v", got.Status, got.PublishedAt)
	}
}
