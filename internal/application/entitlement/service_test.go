package entitlement

import (
	"context"
	"testing"

	domainent "github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	productCreated bool
	granted        bool
	adjusted       bool
}

func (r *fakeRepo) CreateProduct(_ context.Context, _ domainent.CreateProductInput) (*domainent.Product, error) {
	r.productCreated = true
	return &domainent.Product{ID: 1, Status: domainent.ProductStatusActive}, nil
}
func (r *fakeRepo) GetProduct(_ context.Context, _, id int64) (*domainent.Product, error) {
	return &domainent.Product{ID: id}, nil
}
func (r *fakeRepo) ListProducts(_ context.Context, _ domainent.ProductListFilter, _, _ int) ([]*domainent.Product, int64, error) {
	return []*domainent.Product{}, 0, nil
}
func (r *fakeRepo) UpdateProduct(_ context.Context, _, _, id int64, _ domainent.UpdateProductInput) (*domainent.Product, error) {
	return &domainent.Product{ID: id}, nil
}
func (r *fakeRepo) UpdateProductStatus(_ context.Context, _, _, id int64, status string) (*domainent.Product, error) {
	return &domainent.Product{ID: id, Status: domainent.ProductStatus(status)}, nil
}
func (r *fakeRepo) Grant(_ context.Context, _ domainent.GrantInput) (*domainent.Entitlement, error) {
	r.granted = true
	return &domainent.Entitlement{ID: 1, Status: domainent.StatusActive}, nil
}
func (r *fakeRepo) ListEntitlementsByLearner(_ context.Context, _, _ int64) ([]*domainent.Entitlement, error) {
	return []*domainent.Entitlement{}, nil
}
func (r *fakeRepo) GetEntitlement(_ context.Context, _, id int64) (*domainent.Entitlement, error) {
	return &domainent.Entitlement{ID: id}, nil
}
func (r *fakeRepo) Adjust(_ context.Context, _ domainent.AdjustInput) (*domainent.Entitlement, error) {
	r.adjusted = true
	return &domainent.Entitlement{ID: 1}, nil
}
func (r *fakeRepo) SetEntitlementStatus(_ context.Context, _, _, id int64, status, _ string) (*domainent.Entitlement, error) {
	return &domainent.Entitlement{ID: id, Status: domainent.Status(status)}, nil
}
func (r *fakeRepo) ListTransactions(_ context.Context, _, _ int64) ([]*domainent.Transaction, error) {
	return []*domainent.Transaction{}, nil
}

type denyChecker struct{ deny string }

func (c denyChecker) Require(_ context.Context, _, _ int64, code string) error {
	if code == c.deny {
		return apperr.NewAppError(apperr.ErrForbidden, "权限不足", 403)
	}
	return nil
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func packInput() domainent.CreateProductInput {
	return domainent.CreateProductInput{
		BrandID: 1, ActorID: 1, Name: "卡", ProductType: "class_pack",
		TotalCredits: 10, ValidityDays: 90, LocationScope: "all", CourseScope: "all",
	}
}

func TestCreateProduct_PermissionDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{deny: "entitlement.manage"})
	if _, err := s.CreateProduct(context.Background(), packInput()); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.productCreated {
		t.Fatal("should not reach repo when denied")
	}
}

func TestCreateProduct_InvalidType(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := packInput()
	in.ProductType = "spaceship"
	if _, err := s.CreateProduct(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreateProduct_CountBasedNoCredits(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := packInput()
	in.TotalCredits = 0
	if _, err := s.CreateProduct(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM (pack needs credits), got %v", err)
	}
}

func TestCreateProduct_MembershipNoCreditsOK(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)
	in := packInput()
	in.ProductType = "membership_card"
	in.TotalCredits = 0 // 不限次，合法
	if _, err := s.CreateProduct(context.Background(), in); err != nil {
		t.Fatalf("membership without credits should be ok, got %v", err)
	}
	if !repo.productCreated {
		t.Fatal("should reach repo")
	}
}

func TestCreateProduct_InvalidScope(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := packInput()
	in.LocationScope = "foo"
	if _, err := s.CreateProduct(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreateProduct_InvalidValidity(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	in := packInput()
	in.ValidityDays = 0
	if _, err := s.CreateProduct(context.Background(), in); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestGrant_PermissionDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{deny: "entitlement.manage"})
	if _, err := s.Grant(context.Background(), domainent.GrantInput{BrandID: 1, ActorID: 1, LearnerID: 1, ProductID: 1}); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.granted {
		t.Fatal("should not reach repo when denied")
	}
}

func TestAdjust_PermissionDenied(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, denyChecker{deny: "entitlement.adjust"})
	if _, err := s.Adjust(context.Background(), domainent.AdjustInput{BrandID: 1, ActorID: 1, EntitlementID: 1, Delta: 1, Reason: "x"}); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
	if repo.adjusted {
		t.Fatal("should not reach repo when denied")
	}
}

func TestAdjust_ZeroDelta(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	if _, err := s.Adjust(context.Background(), domainent.AdjustInput{BrandID: 1, ActorID: 1, EntitlementID: 1, Delta: 0, Reason: "x"}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestAdjust_NoReason(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	if _, err := s.Adjust(context.Background(), domainent.AdjustInput{BrandID: 1, ActorID: 1, EntitlementID: 1, Delta: 5, Reason: "  "}); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM (reason required), got %v", err)
	}
}

func TestSetStatus_InvalidStatus(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	if _, err := s.SetStatus(context.Background(), 1, 1, 1, "depleted", ""); codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM (only active/frozen/cancelled), got %v", err)
	}
}

func TestListProducts_PermissionDenied(t *testing.T) {
	s := NewService(&fakeRepo{}, denyChecker{deny: "entitlement.view"})
	if _, _, err := s.ListProducts(context.Background(), 1, 1, "", "", 1, 20); codeOf(err) != apperr.ErrForbidden {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
}
