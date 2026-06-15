package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type classSessionRepository struct {
	db *gorm.DB
}

// NewClassSessionRepository 创建场次仓储。
func NewClassSessionRepository(db *gorm.DB) classsession.Repository {
	return &classSessionRepository{db: db}
}

// isExclusionViolation 判断是否 PostgreSQL EXCLUDE 约束冲突（SQLSTATE 23P01）。
// 用于 class_sessions_instructor_no_overlap 触发时映射成业务级冲突错误，而非裸 500。
func isExclusionViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23P01"
	}
	return strings.Contains(err.Error(), "SQLSTATE 23P01")
}

func (r *classSessionRepository) Create(ctx context.Context, in classsession.CreateInput) (*classsession.Session, error) {
	var createdID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) course 属本 brand、未软删、已发布。
		var course CourseTemplateModel
		if err := tx.Where("id = ? AND brand_id = ?", in.CourseID, in.BrandID).First(&course).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
			}
			return apperr.ErrInternalF("查询课程模板失败", err)
		}
		if course.Status != "published" {
			return apperr.NewAppError(apperr.ErrCourseNotActive, "课程模板未发布，无法排课", 409)
		}

		// 2) location 属本 brand 且 active。
		var loc LocationModel
		if err := tx.Where("id = ? AND brand_id = ?", in.LocationID, in.BrandID).First(&loc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
			}
			return apperr.ErrInternalF("查询门店失败", err)
		}
		if loc.Status != "active" {
			return apperr.NewAppError(apperr.ErrCourseLocationUnavailable, "门店已停用，无法排课", 409)
		}
		// 课程在该门店是否可用。
		var availCount int64
		if err := tx.Model(&CourseLocationAvailabilityModel{}).
			Where("course_id = ? AND location_id = ? AND is_available = ?", in.CourseID, in.LocationID, true).
			Count(&availCount).Error; err != nil {
			return apperr.ErrInternalF("查询课程可用门店失败", err)
		}
		if availCount == 0 {
			return apperr.NewAppError(apperr.ErrCourseLocationUnavailable, "该课程在所选门店不可排课", 409)
		}

		// 3) instructor 属本 brand、active、可排课。
		var instr InstructorProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", in.InstructorProfileID, in.BrandID).First(&instr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrInstructorNotSchedulable, "教练不存在或不可排课", 409)
			}
			return apperr.ErrInternalF("查询教练失败", err)
		}
		if instr.Status != "active" || !instr.IsSchedulable {
			return apperr.NewAppError(apperr.ErrInstructorNotSchedulable, "教练不可排课", 409)
		}

		// 4) 容量默认值。
		capacity := in.Capacity
		if capacity <= 0 {
			capacity = course.DefaultCapacity
		}

		// 5) INSERT scheduled（EXCLUDE 约束在 scheduled/in_progress 生效）。
		actor := in.ActorID
		created := ClassSessionModel{
			BrandID:             in.BrandID,
			LocationID:          in.LocationID,
			CourseID:            in.CourseID,
			InstructorProfileID: in.InstructorProfileID,
			StartsAt:            in.StartsAt,
			EndsAt:              in.EndsAt,
			Capacity:            capacity,
			BookedCount:         0,
			WaitlistLimit:       in.WaitlistLimit,
			Status:              string(classsession.StatusScheduled),
		}
		if actor > 0 {
			created.CreatedBy = &actor
		}
		if err := tx.Create(&created).Error; err != nil {
			if isExclusionViolation(err) {
				return apperr.NewAppError(apperr.ErrSessionInstructorConflict, "该教练在此时段已有排课", 409)
			}
			return apperr.ErrInternalF("创建场次失败", err)
		}
		createdID = created.ID
		return writeSessionLog(tx, in.BrandID, in.ActorID, "session_created", created.ID, nil, &created)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, createdID)
}

// sessionRow 反范式扫描行。
type sessionRow struct {
	ClassSessionModel
	CourseTitle    string
	LocationName   string
	InstructorName string
}

func (r *classSessionRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("class_sessions cs").
		Select(`cs.*, c.title AS course_title, l.name AS location_name, ip.display_name AS instructor_name`).
		Joins("JOIN courses c ON c.id = cs.course_id").
		Joins("JOIN locations l ON l.id = cs.location_id").
		Joins("JOIN instructor_profiles ip ON ip.id = cs.instructor_profile_id")
}

func (r *classSessionRepository) GetByID(ctx context.Context, brandID, id int64) (*classsession.Session, error) {
	var row sessionRow
	if err := r.baseQuery(ctx).Where("cs.id = ? AND cs.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询场次失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
	}
	return toSessionDomain(&row), nil
}

func (r *classSessionRepository) List(ctx context.Context, filter classsession.ListFilter, offset, limit int) ([]*classsession.Session, int64, error) {
	q := r.baseQuery(ctx).Where("cs.brand_id = ?", filter.BrandID)
	if filter.LocationID > 0 {
		q = q.Where("cs.location_id = ?", filter.LocationID)
	}
	if filter.CourseID > 0 {
		q = q.Where("cs.course_id = ?", filter.CourseID)
	}
	if filter.InstructorProfileID > 0 {
		q = q.Where("cs.instructor_profile_id = ?", filter.InstructorProfileID)
	}
	if classsession.Status(filter.Status) != "" && isValidSessionStatus(filter.Status) {
		q = q.Where("cs.status = ?", filter.Status)
	}
	if filter.From != nil {
		q = q.Where("cs.starts_at >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("cs.starts_at < ?", *filter.To)
	}
	if filter.ScopeLocationIDs != nil {
		if len(filter.ScopeLocationIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("cs.location_id IN ?", filter.ScopeLocationIDs)
		}
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询场次列表失败", err)
	}

	var rows []sessionRow
	if err := q.Order("cs.starts_at ASC, cs.id ASC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询场次列表失败", err)
	}
	items := make([]*classsession.Session, len(rows))
	for i := range rows {
		items[i] = toSessionDomain(&rows[i])
	}
	return items, total, nil
}

func (r *classSessionRepository) Cancel(ctx context.Context, brandID, actorID, id int64, reason string) (*classsession.Session, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before ClassSessionModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
			}
			return apperr.ErrInternalF("查询场次失败", err)
		}
		if before.Status != string(classsession.StatusScheduled) && before.Status != string(classsession.StatusInProgress) {
			return apperr.NewAppError(apperr.ErrSessionCancelNotAllowed, "仅可取消未开始或进行中的场次", 409)
		}
		after := before
		after.Status = string(classsession.StatusCancelled)
		after.CancelReason = strings.TrimSpace(reason)
		if err := tx.Model(&ClassSessionModel{}).Where("id = ?", id).
			Updates(map[string]interface{}{"status": after.Status, "cancel_reason": after.CancelReason}).Error; err != nil {
			return apperr.ErrInternalF("取消场次失败", err)
		}
		return writeSessionLog(tx, brandID, actorID, "session_cancelled", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func isValidSessionStatus(s string) bool {
	switch classsession.Status(s) {
	case classsession.StatusDraft, classsession.StatusScheduled, classsession.StatusInProgress,
		classsession.StatusCompleted, classsession.StatusCancelled:
		return true
	}
	return false
}

func writeSessionLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *ClassSessionModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "class_session", ID: id},
		Before:  before,
		After:   after,
	})
}

func toSessionDomain(r *sessionRow) *classsession.Session {
	var createdBy *int64
	if r.CreatedBy != nil {
		createdBy = r.CreatedBy
	}
	var resourceID *int64
	if r.LocationResourceID != nil {
		resourceID = r.LocationResourceID
	}
	return &classsession.Session{
		ID:                  r.ID,
		BrandID:             r.BrandID,
		LocationID:          r.LocationID,
		LocationResourceID:  resourceID,
		CourseID:            r.CourseID,
		InstructorProfileID: r.InstructorProfileID,
		StartsAt:            r.StartsAt.UTC(),
		EndsAt:              r.EndsAt.UTC(),
		Capacity:            r.Capacity,
		BookedCount:         r.BookedCount,
		WaitlistLimit:       r.WaitlistLimit,
		Status:              classsession.Status(r.Status),
		CancelReason:        r.CancelReason,
		CreatedBy:           createdBy,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
		CourseTitle:         r.CourseTitle,
		LocationName:        r.LocationName,
		InstructorName:      r.InstructorName,
	}
}
