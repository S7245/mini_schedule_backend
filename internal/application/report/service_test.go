package report

import (
	"context"
	"errors"
	"testing"
	"time"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	domainreport "github.com/zkw/mini-schedule/backend/internal/domain/report"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeChecker struct {
	requireErr error
	scope      domainrbac.DataScope
	resolveErr error
}

func (f fakeChecker) Require(context.Context, int64, int64, string) error { return f.requireErr }
func (f fakeChecker) Resolve(context.Context, int64, int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return domainrbac.PermissionSet{}, f.scope, f.resolveErr
}

type fakeReportRepo struct {
	got    domainreport.ReportQuery
	called bool
}

func (f *fakeReportRepo) BrandOverviewCounts(_ context.Context, q domainreport.ReportQuery) (*domainreport.BrandOverview, error) {
	f.called = true
	f.got = q
	return &domainreport.BrandOverview{}, nil
}

func baseInput() OverviewInput {
	return OverviewInput{BrandID: 21, ActorID: 1, Preset: "this_month"}
}

func TestService_RequireBlocks(t *testing.T) {
	repo := &fakeReportRepo{}
	svc := NewService(repo, fakeChecker{requireErr: errors.New("denied")})
	if _, err := svc.GetBrandOverview(context.Background(), baseInput()); err == nil {
		t.Fatal("expected require error")
	}
	if repo.called {
		t.Error("repo must not be called when permission denied")
	}
}

func TestService_ScopeAllBrand(t *testing.T) {
	repo := &fakeReportRepo{}
	svc := NewService(repo, fakeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAllBrand}})
	if _, err := svc.GetBrandOverview(context.Background(), baseInput()); err != nil {
		t.Fatalf("GetBrandOverview: %v", err)
	}
	if repo.got.ScopeLocationIDs != nil {
		t.Errorf("all_brand scope = %v, want nil", repo.got.ScopeLocationIDs)
	}
}

func TestService_ScopeAssigned(t *testing.T) {
	repo := &fakeReportRepo{}
	svc := NewService(repo, fakeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5, 7}}})
	if _, err := svc.GetBrandOverview(context.Background(), baseInput()); err != nil {
		t.Fatalf("GetBrandOverview: %v", err)
	}
	if len(repo.got.ScopeLocationIDs) != 2 || repo.got.ScopeLocationIDs[0] != 5 {
		t.Errorf("assigned scope = %v, want [5 7]", repo.got.ScopeLocationIDs)
	}
}

func TestService_LocationInScopeGuard(t *testing.T) {
	mk := func() *Service {
		return NewService(&fakeReportRepo{}, fakeChecker{scope: domainrbac.DataScope{Kind: domainrbac.DataScopeAssignedLocations, LocationIDs: []int64{5}}})
	}
	in := baseInput()
	id5, id9 := int64(5), int64(9)
	in.LocationID = &id5
	if _, err := mk().GetBrandOverview(context.Background(), in); err != nil {
		t.Fatalf("location in scope should pass: %v", err)
	}
	in.LocationID = &id9
	_, err := mk().GetBrandOverview(context.Background(), in)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrForbidden {
		t.Errorf("location out of scope = %v, want FORBIDDEN", err)
	}
}

func TestService_NilCheckerAllowsAll(t *testing.T) {
	repo := &fakeReportRepo{}
	svc := NewService(repo, nil)
	if _, err := svc.GetBrandOverview(context.Background(), baseInput()); err != nil {
		t.Fatalf("nil checker should skip gate: %v", err)
	}
	if repo.got.ScopeLocationIDs != nil {
		t.Errorf("nil checker scope = %v, want nil (全品牌)", repo.got.ScopeLocationIDs)
	}
}

func TestResolveWindow(t *testing.T) {
	now := time.Date(2026, 6, 17, 15, 30, 0, 0, time.UTC) // 周三

	check := func(preset, from, to string, wantFrom, wantTo time.Time, wantErr bool) {
		t.Helper()
		gotFrom, gotTo, err := resolveWindow(preset, from, to, now)
		if wantErr {
			if err == nil {
				t.Errorf("%s/%s/%s: expected error", preset, from, to)
			}
			return
		}
		if err != nil {
			t.Fatalf("%s: %v", preset, err)
		}
		if !gotFrom.Equal(wantFrom) || !gotTo.Equal(wantTo) {
			t.Errorf("%s window = [%v,%v), want [%v,%v)", preset, gotFrom, gotTo, wantFrom, wantTo)
		}
	}
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

	check("", "", "", d(2026, 6, 1), d(2026, 7, 1), false)           // 默认 = this_month
	check("this_month", "", "", d(2026, 6, 1), d(2026, 7, 1), false) // 6 月
	check("today", "", "", d(2026, 6, 17), d(2026, 6, 18), false)
	check("this_week", "", "", d(2026, 6, 15), d(2026, 6, 22), false) // 周一 6/15 .. 下周一
	check("custom", "2026-06-01", "2026-06-10", d(2026, 6, 1), d(2026, 6, 11), false)
	check("custom", "", "2026-06-10", time.Time{}, time.Time{}, true)           // 缺 from
	check("custom", "2026-06-10", "2026-06-01", time.Time{}, time.Time{}, true) // to<from
	check("custom", "bad", "2026-06-10", time.Time{}, time.Time{}, true)        // 格式错
	check("nonsense", "", "", time.Time{}, time.Time{}, true)                   // 无效 preset
}
