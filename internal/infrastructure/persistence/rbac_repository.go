package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainrole "github.com/zkw/mini-schedule/backend/internal/domain/role"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type rbacRepository struct {
	db *gorm.DB
}

// NewRBACRepository implements rbac.Repository over the existing brand_users /
// brand_user_role_assignments / brand_roles / brand_role_permissions / permissions
// tables (seeded in migration 000005).
func NewRBACRepository(db *gorm.DB) rbac.Repository {
	return &rbacRepository{db: db}
}

// LoadEffectiveRaw runs a single SQL JOIN across all five tables, then folds
// the rows in Go to dedup permission codes and merge per-assignment data scopes.
//
// Behaviour:
//   - brand_user not found / soft-deleted → STAFF_NOT_FOUND
//   - is_owner=TRUE → returns isOwner=true; rawCodes/scope still reflect what's
//     in role_assignments (for debugging) but Checker uses owner fast-path
//   - non-owner, no active role assignment → empty rawCodes + DataScopeNone
//   - mix of brand-scope + location-scope assignments → MergeScopes folds union
func (r *rbacRepository) LoadEffectiveRaw(
	ctx context.Context, brandID, brandUserID int64,
) ([]string, rbac.DataScope, bool, error) {
	// First confirm the user exists in this brand. Use a tiny SELECT so a 404
	// surfaces before we try the bigger JOIN.
	type ownerRow struct {
		IsOwner bool `gorm:"column:is_owner"`
	}
	var head ownerRow
	if err := r.db.WithContext(ctx).
		Table("brand_users").
		Select("is_owner").
		Where("id = ? AND brand_id = ? AND deleted_at IS NULL", brandUserID, brandID).
		Take(&head).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, rbac.DataScope{Kind: rbac.DataScopeNone}, false,
				apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
		}
		return nil, rbac.DataScope{Kind: rbac.DataScopeNone}, false,
			apperr.ErrInternalF("查询员工失败", err)
	}

	// One pass JOIN for raw codes + per-assignment scope rows.
	type joinRow struct {
		RoleID     *int64  `gorm:"column:role_id"`
		ScopeType  *string `gorm:"column:scope_type"`
		LocationID *int64  `gorm:"column:location_id"`
		DataScope  *string `gorm:"column:data_scope"`
		Code       *string `gorm:"column:code"`
	}
	var rows []joinRow
	if err := r.db.WithContext(ctx).
		Table("brand_user_role_assignments AS bura").
		Select("bura.role_id, br.scope_type, bura.location_id, bura.data_scope, p.code").
		Joins("JOIN brand_roles br ON br.id = bura.role_id AND br.status = 'active'").
		Joins("LEFT JOIN brand_role_permissions brp ON brp.role_id = bura.role_id").
		Joins("LEFT JOIN permissions p ON p.id = brp.permission_id AND p.status = 'active'").
		Where("bura.brand_user_id = ? AND bura.brand_id = ? AND bura.status = 'active'", brandUserID, brandID).
		Scan(&rows).Error; err != nil {
		return nil, rbac.DataScope{Kind: rbac.DataScopeNone}, head.IsOwner,
			apperr.ErrInternalF("查询有效权限失败", err)
	}

	// Aggregate.
	codeSet := map[string]struct{}{}
	type roleAgg struct {
		scopeType   string
		dataScope   string
		locationIDs map[int64]struct{}
	}
	roleMap := map[int64]*roleAgg{}

	for _, row := range rows {
		if row.Code != nil && *row.Code != "" {
			codeSet[*row.Code] = struct{}{}
		}
		if row.RoleID == nil {
			continue
		}
		agg, ok := roleMap[*row.RoleID]
		if !ok {
			st := ""
			if row.ScopeType != nil {
				st = *row.ScopeType
			}
			ds := ""
			if row.DataScope != nil {
				ds = *row.DataScope
			}
			agg = &roleAgg{scopeType: st, dataScope: ds, locationIDs: map[int64]struct{}{}}
			roleMap[*row.RoleID] = agg
		}
		if row.LocationID != nil && *row.LocationID > 0 {
			agg.locationIDs[*row.LocationID] = struct{}{}
		}
	}

	rawCodes := make([]string, 0, len(codeSet))
	for c := range codeSet {
		rawCodes = append(rawCodes, c)
	}

	// Build per-assignment scopes then MergeScopes.
	scopes := make([]rbac.DataScope, 0, len(roleMap))
	for _, agg := range roleMap {
		scopes = append(scopes, deriveScope(agg.scopeType, agg.dataScope, agg.locationIDs))
	}
	merged := rbac.MergeScopes(scopes)

	return rawCodes, merged, head.IsOwner, nil
}

// deriveScope applies the spec table:
//
//	brand-scope + role_default      → DataScopeAllBrand
//	brand-scope + all_brand         → DataScopeAllBrand
//	location-scope + role_default   → DataScopeAssignedLocations(location_ids)
//	location-scope + assigned_locations → DataScopeAssignedLocations(location_ids)
//	own_sessions / own_records      → fallback to assigned_locations (not implemented this batch)
func deriveScope(scopeType, dataScope string, locIDs map[int64]struct{}) rbac.DataScope {
	switch dataScope {
	case domainrole.DataScopeAllBrand:
		return rbac.DataScope{Kind: rbac.DataScopeAllBrand}
	case domainrole.DataScopeAssignedLocations:
		return rbac.DataScope{Kind: rbac.DataScopeAssignedLocations, LocationIDs: mapKeys(locIDs)}
	case domainrole.DataScopeOwnSessions, domainrole.DataScopeOwnRecords:
		// Batch 6 fallback: treat like assigned_locations so users at least don't
		// see all_brand. Logged as a follow-up in FEATURE_REQUESTS.
		return rbac.DataScope{Kind: rbac.DataScopeAssignedLocations, LocationIDs: mapKeys(locIDs)}
	case domainrole.DataScopeRoleDefault, "":
		if scopeType == domainrole.ScopeBrand {
			return rbac.DataScope{Kind: rbac.DataScopeAllBrand}
		}
		return rbac.DataScope{Kind: rbac.DataScopeAssignedLocations, LocationIDs: mapKeys(locIDs)}
	default:
		return rbac.DataScope{Kind: rbac.DataScopeNone}
	}
}

func mapKeys(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ListAllActivePermissionCodes returns every active permission.code. Used by
// rbac.Checker for the owner fast-path catalog.
func (r *rbacRepository) ListAllActivePermissionCodes(ctx context.Context) ([]string, error) {
	var codes []string
	if err := r.db.WithContext(ctx).
		Table("permissions").
		Where("status = ?", "active").
		Order("code ASC").
		Pluck("code", &codes).Error; err != nil {
		return nil, apperr.ErrInternalF("查询权限列表失败", err)
	}
	return codes, nil
}
