package brandprofile

import (
	"context"
	"strings"
	"testing"

	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	profile *persistence.BrandProfile
	updated persistence.UpdateBrandProfileInput
	called  bool
}

func (f *fakeRepo) GetProfile(_ context.Context, _ int64) (*persistence.BrandProfile, error) {
	return f.profile, nil
}

func (f *fakeRepo) UpdateProfile(_ context.Context, _ int64, in persistence.UpdateBrandProfileInput) (*persistence.BrandProfile, error) {
	f.called = true
	f.updated = in
	return f.profile, nil
}

func TestUpdateProfile_RejectsLongDescription(t *testing.T) {
	repo := &fakeRepo{profile: &persistence.BrandProfile{}}
	svc := NewService(repo, nil)
	too := strings.Repeat("a", 2001)
	_, err := svc.UpdateProfile(context.Background(), 1, 0, Input{Description: &too})
	if err == nil {
		t.Fatal("expected INVALID_PARAM")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
	if repo.called {
		t.Errorf("repo should not be called on validation failure")
	}
}

func TestUpdateProfile_RejectsBadEmail(t *testing.T) {
	repo := &fakeRepo{profile: &persistence.BrandProfile{}}
	svc := NewService(repo, nil)
	bad := "not-an-email"
	_, err := svc.UpdateProfile(context.Background(), 1, 0, Input{ContactEmail: &bad})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestUpdateProfile_AcceptsEmptyEmail(t *testing.T) {
	repo := &fakeRepo{profile: &persistence.BrandProfile{}}
	svc := NewService(repo, nil)
	empty := ""
	_, err := svc.UpdateProfile(context.Background(), 1, 0, Input{ContactEmail: &empty})
	if err != nil {
		t.Errorf("empty email should be ok, got %v", err)
	}
}

func TestUpdateProfile_HappyPath_WhitelistFieldsOnly(t *testing.T) {
	repo := &fakeRepo{profile: &persistence.BrandProfile{}}
	svc := NewService(repo, nil)
	desc := "测试"
	industry := "fitness"
	email := "x@y.com"
	logo := "https://example.com/logo.png"
	code := "BR"
	_, err := svc.UpdateProfile(context.Background(), 1, 0, Input{
		Description:  &desc,
		IndustryType: &industry,
		ContactEmail: &email,
		LogoURL:      &logo,
		BrandCode:    &code,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.called {
		t.Fatal("expected repo call")
	}
	if repo.updated.Description == nil || *repo.updated.Description != desc {
		t.Errorf("description not propagated")
	}
	if repo.updated.IndustryType == nil || *repo.updated.IndustryType != industry {
		t.Errorf("industry_type not propagated")
	}
}

// ---- Batch 6: RequirePermission gates ----

type fakeChecker struct {
	requireErrs map[string]error
}

func (f *fakeChecker) Require(_ context.Context, _, _ int64, code string) error {
	if f.requireErrs == nil {
		return nil
	}
	if err, ok := f.requireErrs[code]; ok {
		return err
	}
	return nil
}

func (f *fakeChecker) Resolve(_ context.Context, _, _ int64) (domainrbac.PermissionSet, domainrbac.DataScope, error) {
	return nil, domainrbac.DataScope{}, nil
}

func deniedErr(code string) error {
	return apperr.NewAppError(apperr.ErrPermissionDenied, "权限不足", 403).
		WithDetails(map[string]any{"required": code, "missing": []string{code}})
}

func TestGetProfile_PermissionDenied(t *testing.T) {
	ch := &fakeChecker{requireErrs: map[string]error{"brand.profile.view": deniedErr("brand.profile.view")}}
	svc := NewService(&fakeRepo{}, ch)
	_, err := svc.GetProfile(context.Background(), 1, 18)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestUpdateProfile_PermissionDenied(t *testing.T) {
	ch := &fakeChecker{requireErrs: map[string]error{"brand.profile.edit": deniedErr("brand.profile.edit")}}
	svc := NewService(&fakeRepo{}, ch)
	desc := "hack"
	_, err := svc.UpdateProfile(context.Background(), 1, 18, Input{Description: &desc})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}
