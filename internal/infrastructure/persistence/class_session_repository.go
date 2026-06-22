package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	"github.com/zkw/mini-schedule/backend/internal/domain/locationresource"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type classSessionRepository struct {
	db *gorm.DB
}

// NewClassSessionRepository 创建场次仓储。
func NewClassSessionRepository(db *gorm.DB) classsession.Repository {
	return &classSessionRepository{db: db}
}

// exclusionConstraint 判断是否 PostgreSQL EXCLUDE 约束冲突（SQLSTATE 23P01），
// 并返回触发的约束名。class_sessions 有两条 EXCLUDE（教练时段 / 资源时段），都报 23P01，
// 必须按约束名分流成不同业务错误（SESSION_INSTRUCTOR_CONFLICT vs SESSION_RESOURCE_CONFLICT），
// 而非裸 500。pgErr.ConstraintName 为空时（极少）退化为通用冲突，由调用方按需处理。
func exclusionConstraint(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23P01" {
			return pgErr.ConstraintName, true
		}
		return "", false
	}
	if strings.Contains(err.Error(), "SQLSTATE 23P01") {
		return "", true
	}
	return "", false
}

// sessionConflictError 把 EXCLUDE 约束名映射成对应业务错误。
func sessionConflictError(constraint string) error {
	if constraint == "class_sessions_resource_no_overlap" {
		return apperr.NewAppError(apperr.ErrSessionResourceConflict, "该资源在此时段已被占用", 409)
	}
	return apperr.NewAppError(apperr.ErrSessionInstructorConflict, "该教练在此时段已有排课", 409)
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

		// 4) 可选绑定资源（Batch 12a）：属本 brand + 同 location + active + 未软删。
		//    容量默认值优先级：显式 capacity > 资源容量 > course.default_capacity。
		var resourceCapacity int
		if in.LocationResourceID != nil {
			var res LocationResourceModel
			if err := tx.Where("id = ? AND brand_id = ?", *in.LocationResourceID, in.BrandID).First(&res).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
				}
				return apperr.ErrInternalF("查询资源失败", err)
			}
			if res.LocationID != in.LocationID || res.Status != string(locationresource.StatusActive) {
				return apperr.NewAppError(apperr.ErrResourceNotAvailable, "所选资源已停用或不属于该门店", 409)
			}
			resourceCapacity = res.Capacity
		}

		// 5) 容量默认值。
		capacity := in.Capacity
		if capacity <= 0 {
			if resourceCapacity > 0 {
				capacity = resourceCapacity
			} else {
				capacity = course.DefaultCapacity
			}
		}

		// 6) INSERT scheduled（EXCLUDE 约束在 scheduled/in_progress 生效）。
		actor := in.ActorID
		created := ClassSessionModel{
			BrandID:             in.BrandID,
			LocationID:          in.LocationID,
			LocationResourceID:  in.LocationResourceID,
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
			if constraint, ok := exclusionConstraint(err); ok {
				return sessionConflictError(constraint)
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
	ResourceName   string
}

func (r *classSessionRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("class_sessions cs").
		Select(`cs.*, c.title AS course_title, l.name AS location_name, ip.display_name AS instructor_name, lr.name AS resource_name`).
		Joins("JOIN courses c ON c.id = cs.course_id").
		Joins("JOIN locations l ON l.id = cs.location_id").
		Joins("JOIN instructor_profiles ip ON ip.id = cs.instructor_profile_id").
		Joins("LEFT JOIN location_resources lr ON lr.id = cs.location_resource_id")
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
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Batch 13c：锁场次行，与下单/代取消（同样 SELECT FOR UPDATE 该行）串行化，
		// 确保级联取消进行中无新预约挤入、反之亦然。
		var before ClassSessionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
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

		// Batch 13c 级联：取消所有 active（booked）预约 + 释放权益锁（场次取消恒退，忽略
		// release_on_cancel）+ booked_count 归零。Batch 13d 扩展：再取消活跃候补（见下）。
		var actives []BookingModel
		if err := tx.Where("class_session_id = ? AND status = ?", id, "booked").Find(&actives).Error; err != nil {
			return apperr.ErrInternalF("查询场次预约失败", err)
		}
		cancelledIDs := make([]int64, 0, len(actives))
		sessCancel := "session_cancelled"
		for i := range actives {
			b := actives[i]
			if err := tx.Model(&BookingModel{}).Where("id = ?", b.ID).Updates(map[string]interface{}{
				"status":        "cancelled",
				"cancelled_at":  now,
				"cancelled_by":  actorID,
				"cancel_source": sessCancel,
				"cancel_reason": after.CancelReason,
			}).Error; err != nil {
				return apperr.ErrInternalF("级联取消预约失败", err)
			}
			if err := settleHoldOnCancel(tx, brandID, b.ID, b.BrandLearnerProfileID, actorID, true, now); err != nil {
				return err
			}
			cancelledIDs = append(cancelledIDs, b.ID)
		}

		// Batch 13d 级联：取消活跃候补（waiting/eligible → cancelled）。
		wlRes := tx.Model(&WaitlistEntryModel{}).
			Where("class_session_id = ? AND status IN ('waiting','eligible_to_promote')", id).
			Updates(map[string]interface{}{"status": "cancelled", "operated_by": actorID})
		if wlRes.Error != nil {
			return apperr.ErrInternalF("级联取消候补失败", wlRes.Error)
		}

		updates := map[string]interface{}{"status": after.Status, "cancel_reason": after.CancelReason}
		if len(cancelledIDs) > 0 {
			updates["booked_count"] = 0
			after.BookedCount = 0
		}
		if err := tx.Model(&ClassSessionModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return apperr.ErrInternalF("取消场次失败", err)
		}

		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "session_cancelled",
			Target:  audit.Target{Type: "class_session", ID: id},
			Before:  &before,
			After: map[string]any{
				"status":               after.Status,
				"cancel_reason":        after.CancelReason,
				"cascaded_bookings":    len(cancelledIDs),
				"cascaded_booking_ids": cancelledIDs,
				"cascaded_waitlist":    wlRes.RowsAffected,
			},
		})
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
		ResourceName:        r.ResourceName,
	}
}
