package coursetemplate

import (
	"context"
	"testing"

	domaintpl "github.com/zkw/mini-schedule/backend/internal/domain/coursetemplate"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type fakeRepo struct {
	scheduledCount int64
	deleted        bool
	getErr         error
}

func (r *fakeRepo) Create(_ context.Context, in domaintpl.CreateInput) (*domaintpl.Template, error) {
	return &domaintpl.Template{ID: 1, BrandID: in.BrandID, Title: in.Title, Status: domaintpl.StatusDraft}, nil
}
func (r *fakeRepo) GetByID(_ context.Context, _, id int64) (*domaintpl.Template, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return &domaintpl.Template{ID: id}, nil
}
func (r *fakeRepo) List(_ context.Context, _ domaintpl.ListFilter, _, _ int) ([]*domaintpl.Template, int64, error) {
	return []*domaintpl.Template{}, 0, nil
}
func (r *fakeRepo) Update(_ context.Context, _, _, id int64, _ domaintpl.UpdateInput) (*domaintpl.Template, error) {
	return &domaintpl.Template{ID: id}, nil
}
func (r *fakeRepo) UpdateStatus(_ context.Context, _, _, id int64, st domaintpl.Status) (*domaintpl.Template, error) {
	return &domaintpl.Template{ID: id, Status: st}, nil
}
func (r *fakeRepo) SoftDelete(_ context.Context, _, _, _ int64) error {
	r.deleted = true
	return nil
}
func (r *fakeRepo) CountScheduledSessions(_ context.Context, _, _ int64) (int64, error) {
	return r.scheduledCount, nil
}

func codeOf(err error) apperr.ErrorCode {
	if ae := apperr.GetAppError(err); ae != nil {
		return ae.Code
	}
	return ""
}

func TestCreate_EmptyTitle(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, Title: "  ", DurationMin: 60, DefaultCapacity: 8})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_BadDuration(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, Title: "瑜伽", DurationMin: 0, DefaultCapacity: 8})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestCreate_BadCapacity(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	_, err := s.Create(context.Background(), CreateInput{BrandID: 1, Title: "瑜伽", DurationMin: 60, DefaultCapacity: 0})
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestUpdateStatus_Invalid(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	_, err := s.UpdateStatus(context.Background(), 1, 1, 1, "frozen")
	if codeOf(err) != apperr.ErrInvalidParam {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}

func TestDelete_BlockedWhenScheduled(t *testing.T) {
	repo := &fakeRepo{scheduledCount: 2}
	s := NewService(repo, nil)
	err := s.Delete(context.Background(), 1, 1, 1)
	if codeOf(err) != apperr.ErrCourseInUse {
		t.Fatalf("want COURSE_IN_USE, got %v", err)
	}
	if repo.deleted {
		t.Fatal("should not soft-delete when sessions reference course")
	}
}

func TestDelete_OKWhenNoSessions(t *testing.T) {
	repo := &fakeRepo{scheduledCount: 0}
	s := NewService(repo, nil)
	if err := s.Delete(context.Background(), 1, 1, 1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !repo.deleted {
		t.Fatal("should soft-delete when no sessions reference course")
	}
}
