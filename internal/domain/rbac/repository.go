package rbac

import "context"

// Repository loads the raw effective permission codes + scope for a brand_user.
//
// Implementations should run a single SQL JOIN over
//   brand_users → brand_user_role_assignments → brand_roles → brand_role_permissions → permissions
// returning:
//   - rawCodes: distinct permission.code values from all active role assignments
//   - scope:    merged DataScope (per scope_type + assignment.location_id + data_scope)
//   - isOwner:  brand_users.is_owner == TRUE
//
// When isOwner is true the Checker layer ignores rawCodes/scope and grants the
// owner fast-path (all permissions + DataScopeAllBrand). Implementations are
// still expected to return whatever the JOIN yielded so callers can debug
// assignment drift.
type Repository interface {
	LoadEffectiveRaw(ctx context.Context, brandID, brandUserID int64) (rawCodes []string, scope DataScope, isOwner bool, err error)

	// ListAllActivePermissionCodes returns every active permission.code in the
	// system. Used by Checker to seed the owner fast-path effective set without
	// hard-coding the catalog.
	ListAllActivePermissionCodes(ctx context.Context) ([]string, error)
}
