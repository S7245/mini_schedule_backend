package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/classsession"
	"github.com/zkw/mini-schedule/backend/internal/domain/locationresource"
	domainrec "github.com/zkw/mini-schedule/backend/internal/domain/recurringschedule"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type recurringScheduleRepository struct {
	db *gorm.DB
}

// NewRecurringScheduleRepository 创建循环排课仓储。
func NewRecurringScheduleRepository(db *gorm.DB) domainrec.Repository {
	return &recurringScheduleRepository{db: db}
}

// errAllConflict 哨兵：全部 occurrence 冲突，用于回滚外层 tx（不落空壳 recurring 行）。
var errAllConflict = errors.New("all occurrences conflict")

func (r *recurringScheduleRepository) Generate(ctx context.Context, in domainrec.GenerateInput) (*domainrec.GenerateResult, error) {
	var recID int64
	var skipped []domainrec.SkippedOccurrence
	createdCount := 0

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 批级校验（循环前一次）：course published / 门店 active / 课程在门店可用 / 教练可排课 / 资源可用。
		capacity, err := validateAndResolveCapacity(tx, in)
		if err != nil {
			return err
		}

		// 2) 插 recurring_schedules + weekdays。
		actor := in.ActorID
		rec := RecurringScheduleModel{
			BrandID:             in.BrandID,
			LocationID:          in.LocationID,
			LocationResourceID:  in.LocationResourceID,
			CourseID:            in.CourseID,
			InstructorProfileID: in.InstructorProfileID,
			StartDate:           in.StartDate,
			RepeatWeeks:         in.RepeatWeeks,
			StartTime:           in.StartTime,
			DurationMin:         in.DurationMin,
			Capacity:            capacity,
			Status:              string(domainrec.StatusActive),
		}
		if in.EndDate != "" {
			ed := in.EndDate
			rec.EndDate = &ed
		}
		if actor > 0 {
			rec.CreatedBy = &actor
		}
		if err := tx.Create(&rec).Error; err != nil {
			return apperr.ErrInternalF("创建循环排课失败", err)
		}
		recID = rec.ID
		for _, w := range in.Weekdays {
			if err := tx.Create(&RecurringScheduleWeekdayModel{RecurringScheduleID: rec.ID, Weekday: w}).Error; err != nil {
				return apperr.ErrInternalF("写入循环排课周几失败", err)
			}
		}

		// 3) 逐 occurrence SAVEPOINT 插场次；撞 EXCLUDE → 跳过记清单、继续；其他错误整批 abort。
		for _, o := range in.Occurrences {
			sess := ClassSessionModel{
				BrandID:             in.BrandID,
				LocationID:          in.LocationID,
				LocationResourceID:  in.LocationResourceID,
				CourseID:            in.CourseID,
				InstructorProfileID: in.InstructorProfileID,
				RecurringScheduleID: &rec.ID,
				StartsAt:            o.StartsAt,
				EndsAt:              o.EndsAt,
				Capacity:            capacity,
				Status:              string(classsession.StatusScheduled),
			}
			if actor > 0 {
				sess.CreatedBy = &actor
			}
			serr := tx.Transaction(func(stx *gorm.DB) error {
				return stx.Create(&sess).Error
			})
			if serr != nil {
				if name, ok := exclusionConstraint(serr); ok {
					reason := domainrec.SkipInstructorConflict
					if name == "class_sessions_resource_no_overlap" {
						reason = domainrec.SkipResourceConflict
					}
					skipped = append(skipped, domainrec.SkippedOccurrence{
						Date: o.DateLabel, StartTime: o.TimeLabel, Reason: reason,
					})
					continue
				}
				return apperr.ErrInternalF("生成场次失败", serr)
			}
			createdCount++
		}

		// 4) 0 成功 → 整批回滚（不落空壳）。
		if createdCount == 0 {
			return errAllConflict
		}

		return writeRecurringLog(tx, in.BrandID, in.ActorID, "recurring_schedule_created", rec.ID, nil, &rec)
	})

	if err != nil {
		if errors.Is(err, errAllConflict) {
			return nil, apperr.NewAppError(apperr.ErrRecurringAllConflict, "所选时段全部冲突，未生成任何场次", 409).
				WithDetails(map[string]any{"skipped": skipped})
		}
		return nil, err
	}

	sch, err := r.GetByID(ctx, in.BrandID, recID)
	if err != nil {
		return nil, err
	}
	created, err := r.sessionsOf(ctx, in.BrandID, recID)
	if err != nil {
		return nil, err
	}
	if skipped == nil {
		skipped = []domainrec.SkippedOccurrence{}
	}
	return &domainrec.GenerateResult{Schedule: sch, Created: created, Skipped: skipped}, nil
}

// validateAndResolveCapacity 批级校验 + 解析容量默认值（input > 资源容量 > course.default_capacity）。
func validateAndResolveCapacity(tx *gorm.DB, in domainrec.GenerateInput) (int, error) {
	var course CourseTemplateModel
	if err := tx.Where("id = ? AND brand_id = ?", in.CourseID, in.BrandID).First(&course).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
		}
		return 0, apperr.ErrInternalF("查询课程模板失败", err)
	}
	if course.Status != "published" {
		return 0, apperr.NewAppError(apperr.ErrCourseNotActive, "课程模板未发布，无法排课", 409)
	}

	var loc LocationModel
	if err := tx.Where("id = ? AND brand_id = ?", in.LocationID, in.BrandID).First(&loc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404)
		}
		return 0, apperr.ErrInternalF("查询门店失败", err)
	}
	if loc.Status != "active" {
		return 0, apperr.NewAppError(apperr.ErrCourseLocationUnavailable, "门店已停用，无法排课", 409)
	}
	var availCount int64
	if err := tx.Model(&CourseLocationAvailabilityModel{}).
		Where("course_id = ? AND location_id = ? AND is_available = ?", in.CourseID, in.LocationID, true).
		Count(&availCount).Error; err != nil {
		return 0, apperr.ErrInternalF("查询课程可用门店失败", err)
	}
	if availCount == 0 {
		return 0, apperr.NewAppError(apperr.ErrCourseLocationUnavailable, "该课程在所选门店不可排课", 409)
	}

	var instr InstructorProfileModel
	if err := tx.Where("id = ? AND brand_id = ?", in.InstructorProfileID, in.BrandID).First(&instr).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperr.NewAppError(apperr.ErrInstructorNotSchedulable, "教练不存在或不可排课", 409)
		}
		return 0, apperr.ErrInternalF("查询教练失败", err)
	}
	if instr.Status != "active" || !instr.IsSchedulable {
		return 0, apperr.NewAppError(apperr.ErrInstructorNotSchedulable, "教练不可排课", 409)
	}

	resourceCapacity := 0
	if in.LocationResourceID != nil {
		var res LocationResourceModel
		if err := tx.Where("id = ? AND brand_id = ?", *in.LocationResourceID, in.BrandID).First(&res).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
			}
			return 0, apperr.ErrInternalF("查询资源失败", err)
		}
		if res.LocationID != in.LocationID || res.Status != string(locationresource.StatusActive) {
			return 0, apperr.NewAppError(apperr.ErrResourceNotAvailable, "所选资源已停用或不属于该门店", 409)
		}
		resourceCapacity = res.Capacity
	}

	capacity := in.Capacity
	if capacity <= 0 {
		if resourceCapacity > 0 {
			capacity = resourceCapacity
		} else {
			capacity = course.DefaultCapacity
		}
	}
	return capacity, nil
}

// recurringRow 反范式扫描行（日期/时间用 to_char 投影成字符串）。
type recurringRow struct {
	ID                  int64
	CreatedAt           time.Time
	BrandID             int64
	LocationID          int64
	LocationResourceID  *int64
	CourseID            int64
	InstructorProfileID int64
	StartDate           string
	EndDate             *string
	RepeatWeeks         *int
	StartTime           string
	DurationMin         int
	Capacity            int
	Status              string
	CreatedBy           *int64
	LocationName        string
	CourseTitle         string
	InstructorName      string
	ResourceName        string
	SessionCount        int64
}

func (r *recurringScheduleRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("recurring_schedules rs").
		Select(`rs.id, rs.created_at, rs.brand_id, rs.location_id, rs.location_resource_id,
			rs.course_id, rs.instructor_profile_id,
			to_char(rs.start_date, 'YYYY-MM-DD') AS start_date,
			to_char(rs.end_date, 'YYYY-MM-DD') AS end_date,
			rs.repeat_weeks, to_char(rs.start_time, 'HH24:MI') AS start_time,
			rs.duration_min, rs.capacity, rs.status, rs.created_by,
			l.name AS location_name, c.title AS course_title, ip.display_name AS instructor_name,
			lr.name AS resource_name,
			(SELECT count(*) FROM class_sessions cs WHERE cs.recurring_schedule_id = rs.id) AS session_count`).
		Joins("JOIN locations l ON l.id = rs.location_id").
		Joins("JOIN courses c ON c.id = rs.course_id").
		Joins("JOIN instructor_profiles ip ON ip.id = rs.instructor_profile_id").
		Joins("LEFT JOIN location_resources lr ON lr.id = rs.location_resource_id")
}

func (r *recurringScheduleRepository) weekdaysOf(ctx context.Context, ids []int64) (map[int64][]int, error) {
	out := map[int64][]int{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []RecurringScheduleWeekdayModel
	if err := r.db.WithContext(ctx).
		Where("recurring_schedule_id IN ?", ids).
		Order("recurring_schedule_id, weekday").
		Find(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询循环排课周几失败", err)
	}
	for _, w := range rows {
		out[w.RecurringScheduleID] = append(out[w.RecurringScheduleID], w.Weekday)
	}
	return out, nil
}

func (r *recurringScheduleRepository) sessionsOf(ctx context.Context, brandID, id int64) ([]*classsession.Session, error) {
	cs := &classSessionRepository{db: r.db}
	var rows []sessionRow
	if err := cs.baseQuery(ctx).
		Where("cs.recurring_schedule_id = ? AND cs.brand_id = ?", id, brandID).
		Order("cs.starts_at ASC, cs.id ASC").Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询循环排课场次失败", err)
	}
	items := make([]*classsession.Session, len(rows))
	for i := range rows {
		items[i] = toSessionDomain(&rows[i])
	}
	return items, nil
}

func (r *recurringScheduleRepository) GetByID(ctx context.Context, brandID, id int64) (*domainrec.Schedule, error) {
	var row recurringRow
	if err := r.baseQuery(ctx).Where("rs.id = ? AND rs.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询循环排课失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrRecurringNotFound, "循环排课不存在", 404)
	}
	wmap, err := r.weekdaysOf(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	return toRecurringDomain(&row, wmap[id]), nil
}

func (r *recurringScheduleRepository) GetDetail(ctx context.Context, brandID, id int64) (*domainrec.Schedule, []*classsession.Session, error) {
	sch, err := r.GetByID(ctx, brandID, id)
	if err != nil {
		return nil, nil, err
	}
	sessions, err := r.sessionsOf(ctx, brandID, id)
	if err != nil {
		return nil, nil, err
	}
	return sch, sessions, nil
}

func (r *recurringScheduleRepository) List(ctx context.Context, filter domainrec.ListFilter, offset, limit int) ([]*domainrec.Schedule, int64, error) {
	q := r.baseQuery(ctx).Where("rs.brand_id = ?", filter.BrandID)
	if filter.LocationID > 0 {
		q = q.Where("rs.location_id = ?", filter.LocationID)
	}
	if domainrec.IsValidStatus(filter.Status) {
		q = q.Where("rs.status = ?", filter.Status)
	}
	if filter.ScopeLocationIDs != nil {
		if len(filter.ScopeLocationIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("rs.location_id IN ?", filter.ScopeLocationIDs)
		}
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询循环排课列表失败", err)
	}

	var rows []recurringRow
	if err := q.Order("rs.id DESC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询循环排课列表失败", err)
	}
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	wmap, err := r.weekdaysOf(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*domainrec.Schedule, len(rows))
	for i := range rows {
		items[i] = toRecurringDomain(&rows[i], wmap[rows[i].ID])
	}
	return items, total, nil
}

func (r *recurringScheduleRepository) Cancel(ctx context.Context, brandID, actorID, id int64) (*domainrec.Schedule, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before RecurringScheduleModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrRecurringNotFound, "循环排课不存在", 404)
			}
			return apperr.ErrInternalF("查询循环排课失败", err)
		}
		if before.Status != string(domainrec.StatusActive) {
			return apperr.NewAppError(apperr.ErrRecurringCancelNotAllowed, "仅可取消进行中的循环排课", 409)
		}
		after := before
		after.Status = string(domainrec.StatusCancelled)
		if err := tx.Model(&RecurringScheduleModel{}).Where("id = ? AND brand_id = ?", id, brandID).
			Update("status", after.Status).Error; err != nil {
			return apperr.ErrInternalF("取消循环排课失败", err)
		}
		// 非级联：不动已生成场次。
		return writeRecurringLog(tx, brandID, actorID, "recurring_schedule_cancelled", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func writeRecurringLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *RecurringScheduleModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "recurring_schedule", ID: id},
		Before:  before,
		After:   after,
	})
}

func toRecurringDomain(r *recurringRow, weekdays []int) *domainrec.Schedule {
	if weekdays == nil {
		weekdays = []int{}
	}
	endDate := ""
	if r.EndDate != nil {
		endDate = *r.EndDate
	}
	return &domainrec.Schedule{
		ID:                  r.ID,
		BrandID:             r.BrandID,
		LocationID:          r.LocationID,
		LocationResourceID:  r.LocationResourceID,
		CourseID:            r.CourseID,
		InstructorProfileID: r.InstructorProfileID,
		Weekdays:            weekdays,
		StartDate:           r.StartDate,
		EndDate:             endDate,
		RepeatWeeks:         r.RepeatWeeks,
		StartTime:           r.StartTime,
		DurationMin:         r.DurationMin,
		Capacity:            r.Capacity,
		Status:              domainrec.Status(r.Status),
		CreatedBy:           r.CreatedBy,
		CreatedAt:           r.CreatedAt,
		LocationName:        r.LocationName,
		CourseTitle:         r.CourseTitle,
		InstructorName:      r.InstructorName,
		ResourceName:        r.ResourceName,
		SessionCount:        r.SessionCount,
	}
}
