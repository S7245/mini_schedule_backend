package coursecategory

import (
	"context"
	"errors"
	"strings"
	"testing"

	domaincat "github.com/zkw/mini-schedule/backend/internal/domain/coursecategory"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// fakeRepo 手写 in-memory repository，覆盖 service 单测路径。
type fakeRepo struct {
	createCalls []domaincat.CreateInput
	updateCalls []domaincat.UpdateInput
	listFilter  domaincat.ListFilter
	createErr   error
}

func (r *fakeRepo) Create(_ context.Context, in domaincat.CreateInput) (*domaincat.Category, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.createCalls = append(r.createCalls, in)
	return &domaincat.Category{ID: 1, BrandID: in.BrandID, Name: in.Name, Status: domaincat.StatusActive}, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, _ int64) (*domaincat.Category, error) {
	return &domaincat.Category{ID: 1}, nil
}
func (r *fakeRepo) List(_ context.Context, f domaincat.ListFilter) ([]*domaincat.Category, error) {
	r.listFilter = f
	return []*domaincat.Category{}, nil
}
func (r *fakeRepo) Update(_ context.Context, _, _, _ int64, in domaincat.UpdateInput) (*domaincat.Category, error) {
	r.updateCalls = append(r.updateCalls, in)
	return &domaincat.Category{ID: 1}, nil
}
func (r *fakeRepo) CountActiveByIDs(_ context.Context, _ int64, ids []int64) (int64, error) {
	return int64(len(ids)), nil
}

// denyChecker 永远拒绝，用于断言 require 透传。
type denyChecker struct{}

func (denyChecker) Require(_ context.Context, _, _ int64, code string) error {
	return apperr.NewAppError(apperr.ErrPermissionDenied, "denied:"+code, 403)
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func TestCreate_RequireDenied(t *testing.T) {
	s := NewService(&fakeRepo{}, denyChecker{})
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, Name: "团课"})
	if codeOf(err) != apperr.ErrPermissionDenied {
		t.Fatalf("want PERMISSION_DENIED, got %v", err)
	}
}

func TestCreate_EmptyName(t *testing.T) {
	s := NewService(&fakeRepo{}, nil) // checker nil = bypass
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, Name: "  "})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_NameTooLong(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, Name: strings.Repeat("字", 101)})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_TrimsAndDelegates(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)
	_, err := s.Create(context.Background(), CreateInput{BrandID: 7, Name: "  团课  "})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(repo.createCalls) != 1 || repo.createCalls[0].Name != "团课" {
		t.Fatalf("name should be trimmed before repo, got %+v", repo.createCalls)
	}
}

func TestCreate_PropagatesRepoError(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("boom")}
	s := NewService(repo, nil)
	if _, err := s.Create(context.Background(), CreateInput{BrandID: 1, Name: "x"}); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

func TestUpdate_InvalidStatus(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	bad := "frozen"
	_, err := s.Update(context.Background(), 1, 1, 1, UpdateInput{Status: &bad})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestUpdate_EmptyName(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	empty := "   "
	_, err := s.Update(context.Background(), 1, 1, 1, UpdateInput{Name: &empty})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestList_AllMapsToEmptyStatus(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)
	if _, err := s.List(context.Background(), 1, 1, "all"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if repo.listFilter.Status != "" {
		t.Fatalf(`status "all" should map to "", got %q`, repo.listFilter.Status)
	}
}
