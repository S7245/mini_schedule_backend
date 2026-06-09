package location

import (
	"context"
	"errors"
	"testing"
	"time"

	domainlocation "github.com/zkw/mini-schedule/backend/internal/domain/location"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	created  *domainlocation.Location
	createErr error

	updateErr error
	statusErr error
	delErr    error

	getErr   error
	got      *domainlocation.Location

	listItems []*domainlocation.Location
	listTotal int64
	listErr   error

	createCalled bool
	createIn     domainlocation.CreateLocationInput
}

func (f *fakeRepo) Create(_ context.Context, in domainlocation.CreateLocationInput) (*domainlocation.Location, error) {
	f.createCalled = true
	f.createIn = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.created, nil
}

func (f *fakeRepo) GetByID(_ context.Context, _, _ int64) (*domainlocation.Location, error) {
	return f.got, f.getErr
}

func (f *fakeRepo) List(_ context.Context, _ domainlocation.ListLocationsFilter, _, _ int) ([]*domainlocation.Location, int64, error) {
	return f.listItems, f.listTotal, f.listErr
}

func (f *fakeRepo) Update(_ context.Context, _, _ int64, _ domainlocation.UpdateLocationInput) (*domainlocation.Location, error) {
	return f.got, f.updateErr
}

func (f *fakeRepo) UpdateStatus(_ context.Context, _, _, _ int64, _ domainlocation.Status) (*domainlocation.Location, error) {
	return f.got, f.statusErr
}

func (f *fakeRepo) SoftDelete(_ context.Context, _, _, _ int64) error {
	return f.delErr
}

func TestCreate_EmptyName(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Name: " "})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
	if repo.createCalled {
		t.Errorf("repo should not be invoked on validation failure")
	}
}

func TestCreate_PropagatesSubscriptionRestricted(t *testing.T) {
	repo := &fakeRepo{
		createErr: apperr.NewAppError(apperr.ErrSubscriptionRestricted, "no sub", 403),
	}
	svc := NewService(repo, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Name: "总店"})
	if err == nil {
		t.Fatal("expected error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrSubscriptionRestricted {
		t.Errorf("expected SUBSCRIPTION_RESTRICTED, got %v", err)
	}
}

func TestCreate_PropagatesQuotaExceeded(t *testing.T) {
	repo := &fakeRepo{
		createErr: apperr.NewAppError(apperr.ErrQuotaExceeded, "max reached", 409),
	}
	svc := NewService(repo, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Name: "新店"})
	if err == nil {
		t.Fatal("expected error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrQuotaExceeded {
		t.Errorf("expected QUOTA_EXCEEDED, got %v", err)
	}
}

func TestCreate_HappyPath(t *testing.T) {
	repo := &fakeRepo{
		created: &domainlocation.Location{ID: 1, BrandID: 1, Name: "总店", Status: domainlocation.StatusActive, CreatedAt: time.Now()},
	}
	svc := NewService(repo, nil)
	got, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Name: "总店"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "总店" {
		t.Errorf("name mismatch: %s", got.Name)
	}
	if !repo.createCalled {
		t.Errorf("expected repo.Create to be called")
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: apperr.NewAppError(apperr.ErrLocationNotFound, "missing", 404)}
	svc := NewService(repo, nil)
	_, err := svc.Get(context.Background(), 1, 0, 999)
	if err == nil {
		t.Fatal("expected error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrLocationNotFound {
		t.Errorf("expected LOCATION_NOT_FOUND, got %v", err)
	}
}

func TestUpdateStatus_RejectsBadStatus(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)
	_, err := svc.UpdateStatus(context.Background(), 1, 99, 1, "frozen")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrInvalidParam {
		t.Errorf("expected INVALID_PARAM, got %v", err)
	}
}

func TestList_Pagination(t *testing.T) {
	repo := &fakeRepo{listItems: []*domainlocation.Location{{ID: 1}}, listTotal: 1}
	svc := NewService(repo, nil)
	items, total, err := svc.List(context.Background(), ListInput{BrandID: 1, Page: 0, PageSize: 0, Status: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("unexpected list output")
	}
}

func TestDelete_NotFoundPropagates(t *testing.T) {
	repo := &fakeRepo{delErr: apperr.NewAppError(apperr.ErrLocationNotFound, "x", 404)}
	svc := NewService(repo, nil)
	err := svc.Delete(context.Background(), 1, 99, 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrLocationNotFound {
		t.Errorf("expected LOCATION_NOT_FOUND, got %v", err)
	}
}

func TestCreate_GenericError(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("db down")}
	svc := NewService(repo, nil)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected error")
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

func TestCreate_PermissionDeniedNoLocCreate(t *testing.T) {
	ch := &fakeChecker{requireErrs: map[string]error{"location.create": deniedErr("location.create")}}
	repo := &fakeRepo{}
	svc := NewService(repo, ch)
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, ActorID: 18, Name: "Loc"})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestList_PermissionDeniedNoLocView(t *testing.T) {
	ch := &fakeChecker{requireErrs: map[string]error{"location.view": deniedErr("location.view")}}
	repo := &fakeRepo{}
	svc := NewService(repo, ch)
	_, _, err := svc.List(context.Background(), ListInput{BrandID: 1, ActorID: 18})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestDelete_PermissionDeniedNoLocDelete(t *testing.T) {
	ch := &fakeChecker{requireErrs: map[string]error{"location.delete": deniedErr("location.delete")}}
	repo := &fakeRepo{}
	svc := NewService(repo, ch)
	err := svc.Delete(context.Background(), 1, 18, 1)
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}

func TestUpdate_PermissionDeniedNoLocEdit(t *testing.T) {
	ch := &fakeChecker{requireErrs: map[string]error{"location.edit": deniedErr("location.edit")}}
	repo := &fakeRepo{}
	svc := NewService(repo, ch)
	_, err := svc.Update(context.Background(), 1, 18, 1, UpdateInput{})
	if ae := apperr.GetAppError(err); ae == nil || ae.Code != apperr.ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %v", err)
	}
}
