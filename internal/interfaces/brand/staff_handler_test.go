package brand

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/zkw/mini-schedule/backend/internal/domain/role"
)

// TestRegisterRoutes_NoPanic ensures the staff + role + permission routes register
// without Gin path-conflict panics (e.g. mixed param names on a shared prefix).
func TestRegisterRoutes_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	g := e.Group("/api/v1/brand")
	h := NewStaffHandler(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterRoutes panicked: %v", r)
		}
	}()
	h.RegisterRoutes(g)

	want := map[string]bool{
		"GET /api/v1/brand/permissions":        false,
		"POST /api/v1/brand/roles":             false,
		"GET /api/v1/brand/roles/:id":          false,
		"PUT /api/v1/brand/roles/:id":          false,
		"PATCH /api/v1/brand/roles/:id/status": false,
		"DELETE /api/v1/brand/roles/:id":       false,
	}
	for _, ri := range e.Routes() {
		key := ri.Method + " " + ri.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for k, found := range want {
		if !found {
			t.Errorf("route not registered: %s", k)
		}
	}
}

func TestGroupPermissionsByDomain(t *testing.T) {
	perms := []role.Permission{
		{Code: "brand.profile.view", Domain: "brand", Action: "view", Name: "查看品牌资料", Description: "d1"},
		{Code: "brand.profile.edit", Domain: "brand", Action: "edit", Name: "编辑品牌资料"},
		{Code: "role.manage", Domain: "role", Action: "manage", Name: "角色管理"},
		{Code: "staff.create", Domain: "staff", Action: "create", Name: "新增员工"},
	}
	groups := groupPermissionsByDomain(perms)

	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3 (brand, role, staff)", len(groups))
	}
	// Order preserved from input (service pre-sorts by domain, code).
	if groups[0].Domain != "brand" || len(groups[0].Permissions) != 2 {
		t.Fatalf("group[0] = %+v, want brand with 2 perms", groups[0])
	}
	if groups[0].Permissions[0].Code != "brand.profile.view" || groups[0].Permissions[0].Description != "d1" {
		t.Fatalf("brand first perm = %+v", groups[0].Permissions[0])
	}
	if groups[1].Domain != "role" || groups[1].Permissions[0].Code != "role.manage" {
		t.Fatalf("group[1] = %+v, want role/role.manage", groups[1])
	}
}

func TestGroupPermissionsByDomain_Empty(t *testing.T) {
	groups := groupPermissionsByDomain(nil)
	if groups == nil {
		t.Fatalf("expected non-nil empty slice for [] JSON serialization")
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
	}
}
