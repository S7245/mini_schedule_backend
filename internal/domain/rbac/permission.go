// Package rbac defines the RBAC domain types — permission sets with implied-permission
// expansion and data-scope merging used across application services and the brand /me
// endpoint.
//
// Implied permission rules (per Batch 6 spec §"permission inheritance"):
//
//	X.edit   → +X.view
//	X.create → +X.view
//	X.delete → +X.view, +X.edit
//
// implication is computed in-memory only; permission tables stay normalized.
package rbac

import "strings"

// PermissionSet is an unordered set of permission codes (e.g. "staff.view").
type PermissionSet map[string]struct{}

// Has returns true if code is in the set.
func (s PermissionSet) Has(code string) bool {
	if s == nil {
		return false
	}
	_, ok := s[code]
	return ok
}

// HasAll returns true if every code is in the set.
func (s PermissionSet) HasAll(codes ...string) bool {
	if s == nil {
		return len(codes) == 0
	}
	for _, c := range codes {
		if _, ok := s[c]; !ok {
			return false
		}
	}
	return true
}

// Codes returns the codes as a slice (unsorted; callers sort if needed).
func (s PermissionSet) Codes() []string {
	if len(s) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(s))
	for c := range s {
		out = append(out, c)
	}
	return out
}

// Expand returns a new PermissionSet containing raw codes plus implied codes per
// the rules above. Input slice is never mutated. Unknown action suffixes are
// passed through untouched.
func Expand(raw []string) PermissionSet {
	out := make(PermissionSet, len(raw)*2)
	for _, code := range raw {
		if code == "" {
			continue
		}
		out[code] = struct{}{}
		dot := strings.LastIndex(code, ".")
		if dot < 0 {
			continue
		}
		prefix := code[:dot]
		action := code[dot+1:]
		switch action {
		case "edit":
			out[prefix+".view"] = struct{}{}
		case "create":
			out[prefix+".view"] = struct{}{}
		case "delete":
			out[prefix+".view"] = struct{}{}
			out[prefix+".edit"] = struct{}{}
		}
	}
	return out
}

// DataScopeKind is the effective data scope a user has after merging assignments.
type DataScopeKind string

const (
	// DataScopeAllBrand sees the entire brand.
	DataScopeAllBrand DataScopeKind = "all_brand"
	// DataScopeAssignedLocations is restricted to a list of location_ids.
	DataScopeAssignedLocations DataScopeKind = "assigned_locations"
	// DataScopeNone has no scope — every query must be rejected (except self-service).
	DataScopeNone DataScopeKind = "none"
)

// DataScope is the post-merge effective scope. LocationIDs is populated only when
// Kind == DataScopeAssignedLocations.
type DataScope struct {
	Kind        DataScopeKind `json:"kind"`
	LocationIDs []int64       `json:"location_ids,omitempty"`
}

// MergeScopes folds multiple per-assignment scopes into one. Rules:
//   - empty input → DataScopeNone
//   - any DataScopeAllBrand → DataScopeAllBrand (location_ids cleared)
//   - otherwise → union of DataScopeAssignedLocations location_ids
//   - DataScopeNone entries are skipped (treated as "no opinion")
//
// If every input is DataScopeNone, output is DataScopeNone.
func MergeScopes(scopes []DataScope) DataScope {
	if len(scopes) == 0 {
		return DataScope{Kind: DataScopeNone}
	}
	hasAny := false
	seen := map[int64]struct{}{}
	for _, s := range scopes {
		switch s.Kind {
		case DataScopeAllBrand:
			return DataScope{Kind: DataScopeAllBrand}
		case DataScopeAssignedLocations:
			hasAny = true
			for _, id := range s.LocationIDs {
				if id > 0 {
					seen[id] = struct{}{}
				}
			}
		}
	}
	if !hasAny {
		return DataScope{Kind: DataScopeNone}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return DataScope{Kind: DataScopeAssignedLocations, LocationIDs: ids}
}
