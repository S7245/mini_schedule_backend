package persistence

import (
	"context"
	"testing"
)

// TestStaffGetByID_EmbedsInstructorProfile 锁定：员工已建教练档案时，GET /staff/:id
// 详情必须内嵌 instructor_profile（缺这块导致前端「教练档案」卡显示「未启用」）。
func TestStaffGetByID_EmbedsInstructorProfile(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewStaffRepository(db, nil)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	instrProfileID := seedInstructor(t, db, brandID) // 同时建 brand_user + instructor_profile
	// 取该 instructor profile 对应的 brand_user_id
	var brandUserID int64
	if err := db.Raw(`SELECT brand_user_id FROM instructor_profiles WHERE id = ?`, instrProfileID).
		Scan(&brandUserID).Error; err != nil {
		t.Fatalf("read brand_user_id: %v", err)
	}

	got, err := repo.GetWithAssignments(context.Background(), brandID, brandUserID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.HasInstructor {
		t.Errorf("has_instructor = false, want true")
	}
	if got.InstructorProfile == nil {
		t.Fatalf("instructor_profile = nil, want embedded profile (this is the /staff/:id 未启用 bug)")
	}
	if got.InstructorProfile.Status != "active" || !got.InstructorProfile.IsSchedulable {
		t.Errorf("embedded profile = %+v, want active+schedulable", got.InstructorProfile)
	}
	if got.InstructorProfile.Specialties == nil || got.InstructorProfile.Certificates == nil {
		t.Errorf("specialties/certificates must be [] not nil (前端 .map 防御)")
	}
}

// TestStaffGetByID_NoProfileNilEmbed 无教练档案时 instructor_profile 为 nil（序列化 null）。
func TestStaffGetByID_NoProfileNilEmbed(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := NewStaffRepository(db, nil)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	brandUserID := seedBrandUser(t, db, brandID) // 普通员工，无教练档案

	got, err := repo.GetWithAssignments(context.Background(), brandID, brandUserID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.HasInstructor {
		t.Errorf("has_instructor = true, want false")
	}
	if got.InstructorProfile != nil {
		t.Errorf("instructor_profile = %+v, want nil", got.InstructorProfile)
	}
}
