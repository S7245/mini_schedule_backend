package brandprofile

import (
	"context"
	"strings"
	"testing"

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
	svc := NewService(repo)
	too := strings.Repeat("a", 2001)
	_, err := svc.UpdateProfile(context.Background(), 1, Input{Description: &too})
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
	svc := NewService(repo)
	bad := "not-an-email"
	_, err := svc.UpdateProfile(context.Background(), 1, Input{ContactEmail: &bad})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestUpdateProfile_AcceptsEmptyEmail(t *testing.T) {
	repo := &fakeRepo{profile: &persistence.BrandProfile{}}
	svc := NewService(repo)
	empty := ""
	_, err := svc.UpdateProfile(context.Background(), 1, Input{ContactEmail: &empty})
	if err != nil {
		t.Errorf("empty email should be ok, got %v", err)
	}
}

func TestUpdateProfile_HappyPath_WhitelistFieldsOnly(t *testing.T) {
	repo := &fakeRepo{profile: &persistence.BrandProfile{}}
	svc := NewService(repo)
	desc := "测试"
	industry := "fitness"
	email := "x@y.com"
	logo := "https://example.com/logo.png"
	code := "BR"
	_, err := svc.UpdateProfile(context.Background(), 1, Input{
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
