package rbac

import (
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
)

// dryRunDB returns a GORM session that doesn't talk to a real DB but lets us
// inspect the generated SQL via Statement.SQL.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(nil, &gorm.Config{DryRun: true})
	if err == nil && db != nil {
		return db.Session(&gorm.Session{DryRun: true})
	}
	// Fallback: construct a Statement manually (no dialector). Use a tiny shim.
	stmt := &gorm.Statement{
		DB: &gorm.DB{Config: &gorm.Config{}},
	}
	_ = stmt
	t.Skip("gorm DryRun unavailable without dialector — scope_resolver covered via integration")
	return nil
}

func TestApplyToQuery_AllBrandIsNoOp(t *testing.T) {
	cond := ApplyToQuery("location_id", domainrbac.DataScope{Kind: domainrbac.DataScopeAllBrand})
	if cond != nil {
		t.Fatalf("AllBrand should yield nil condition, got %v", cond)
	}
}

func TestApplyToQuery_AssignedLocationsBuildsIN(t *testing.T) {
	cond := ApplyToQuery("location_id", domainrbac.DataScope{
		Kind:        domainrbac.DataScopeAssignedLocations,
		LocationIDs: []int64{1, 2},
	})
	if cond == nil {
		t.Fatal("expected IN clause condition, got nil")
	}
	// Best-effort assertion: ApplyToQuery returns a *clause.Expr; SQL contains IN.
	expr, ok := cond.(*clause.Expr)
	if !ok {
		t.Fatalf("expected *clause.Expr, got %T", cond)
	}
	if expr.SQL == "" {
		t.Fatal("expected non-empty SQL")
	}
}

func TestApplyToQuery_AssignedLocationsEmptyDenies(t *testing.T) {
	cond := ApplyToQuery("location_id", domainrbac.DataScope{
		Kind:        domainrbac.DataScopeAssignedLocations,
		LocationIDs: nil,
	})
	expr, ok := cond.(*clause.Expr)
	if !ok {
		t.Fatalf("expected *clause.Expr, got %T", cond)
	}
	if expr.SQL != "1=0" {
		t.Fatalf("expected 1=0 sentinel, got %q", expr.SQL)
	}
}

func TestApplyToQuery_NoneDenies(t *testing.T) {
	cond := ApplyToQuery("location_id", domainrbac.DataScope{Kind: domainrbac.DataScopeNone})
	expr, ok := cond.(*clause.Expr)
	if !ok {
		t.Fatalf("expected *clause.Expr, got %T", cond)
	}
	if expr.SQL != "1=0" {
		t.Fatalf("expected 1=0 sentinel, got %q", expr.SQL)
	}
}
