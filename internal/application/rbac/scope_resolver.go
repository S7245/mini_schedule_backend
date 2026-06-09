package rbac

import (
	"gorm.io/gorm/clause"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
)

// ApplyToQuery translates a DataScope into a GORM `clause.Expression` ready
// to be chained via `q.Where(cond)`. Returns nil when no filtering is needed
// (DataScopeAllBrand).
//
// Examples:
//
//	cond := rbac.ApplyToQuery("staff_location_assignments.location_id", scope)
//	if cond != nil { q = q.Where(cond) }
//
// Behaviour:
//   - AllBrand → nil (no-op; let caller skip)
//   - AssignedLocations, len>0 → "<col> IN (?)"
//   - AssignedLocations, len==0 → "1=0" sentinel (always-false; safer than no-op)
//   - None → "1=0" sentinel
func ApplyToQuery(col string, scope domainrbac.DataScope) clause.Expression {
	switch scope.Kind {
	case domainrbac.DataScopeAllBrand:
		return nil
	case domainrbac.DataScopeAssignedLocations:
		if len(scope.LocationIDs) == 0 {
			return &clause.Expr{SQL: "1=0"}
		}
		return &clause.Expr{
			SQL:  col + " IN (?)",
			Vars: []any{scope.LocationIDs},
		}
	default:
		// DataScopeNone or unknown → fail closed.
		return &clause.Expr{SQL: "1=0"}
	}
}
