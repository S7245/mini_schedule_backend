package rbac

import (
	"reflect"
	"sort"
	"testing"
)

func TestExpand_EditImpliesView(t *testing.T) {
	got := Expand([]string{"staff.edit"})
	if !got.Has("staff.edit") || !got.Has("staff.view") {
		t.Fatalf("edit should imply view, got %v", got)
	}
}

func TestExpand_CreateImpliesView(t *testing.T) {
	got := Expand([]string{"staff.create"})
	if !got.Has("staff.create") || !got.Has("staff.view") {
		t.Fatalf("create should imply view, got %v", got)
	}
}

func TestExpand_DeleteImpliesViewAndEdit(t *testing.T) {
	got := Expand([]string{"staff.delete"})
	if !got.Has("staff.delete") || !got.Has("staff.view") || !got.Has("staff.edit") {
		t.Fatalf("delete should imply view + edit, got %v", got)
	}
}

func TestExpand_IdempotentAndDeduplicates(t *testing.T) {
	got := Expand([]string{"staff.edit", "staff.view", "staff.edit"})
	if len(got) != 2 {
		t.Fatalf("expected dedup to 2 codes, got %v", got)
	}
}

func TestExpand_DoesNotMutateInput(t *testing.T) {
	in := []string{"staff.delete"}
	_ = Expand(in)
	if !reflect.DeepEqual(in, []string{"staff.delete"}) {
		t.Fatalf("Expand mutated input: %v", in)
	}
}

func TestExpand_PreservesUnrelatedCodes(t *testing.T) {
	got := Expand([]string{"instructor.edit", "location.delete"})
	want := []string{"instructor.edit", "instructor.view", "location.delete", "location.edit", "location.view"}
	gotList := got.Codes()
	sort.Strings(gotList)
	sort.Strings(want)
	if !reflect.DeepEqual(gotList, want) {
		t.Fatalf("expected %v, got %v", want, gotList)
	}
}

func TestPermissionSet_HasAll(t *testing.T) {
	ps := Expand([]string{"staff.delete"})
	if !ps.HasAll("staff.view", "staff.edit", "staff.delete") {
		t.Fatalf("HasAll should pass, got %v", ps)
	}
	if ps.HasAll("staff.create") {
		t.Fatalf("should not have staff.create")
	}
}

func TestMergeScopes_Empty(t *testing.T) {
	out := MergeScopes(nil)
	if out.Kind != DataScopeNone {
		t.Fatalf("empty scopes → DataScopeNone, got %v", out.Kind)
	}
}

func TestMergeScopes_AnyAllBrandWins(t *testing.T) {
	out := MergeScopes([]DataScope{
		{Kind: DataScopeAssignedLocations, LocationIDs: []int64{1}},
		{Kind: DataScopeAllBrand},
	})
	if out.Kind != DataScopeAllBrand {
		t.Fatalf("any all_brand → all_brand, got %v", out.Kind)
	}
	if len(out.LocationIDs) != 0 {
		t.Fatalf("all_brand should drop location_ids, got %v", out.LocationIDs)
	}
}

func TestMergeScopes_UnionLocations(t *testing.T) {
	out := MergeScopes([]DataScope{
		{Kind: DataScopeAssignedLocations, LocationIDs: []int64{1, 2}},
		{Kind: DataScopeAssignedLocations, LocationIDs: []int64{2, 3}},
	})
	if out.Kind != DataScopeAssignedLocations {
		t.Fatalf("expected DataScopeAssignedLocations, got %v", out.Kind)
	}
	got := append([]int64(nil), out.LocationIDs...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("expected union [1,2,3], got %v", got)
	}
}

func TestMergeScopes_NoneSkipped(t *testing.T) {
	out := MergeScopes([]DataScope{
		{Kind: DataScopeNone},
		{Kind: DataScopeAssignedLocations, LocationIDs: []int64{5}},
	})
	if out.Kind != DataScopeAssignedLocations {
		t.Fatalf("expected DataScopeAssignedLocations, got %v", out.Kind)
	}
	if !reflect.DeepEqual(out.LocationIDs, []int64{5}) {
		t.Fatalf("expected [5], got %v", out.LocationIDs)
	}
}

func TestMergeScopes_AllNone(t *testing.T) {
	out := MergeScopes([]DataScope{{Kind: DataScopeNone}, {Kind: DataScopeNone}})
	if out.Kind != DataScopeNone {
		t.Fatalf("expected DataScopeNone, got %v", out.Kind)
	}
}
