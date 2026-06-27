package persistence

import (
	"context"
	"strings"

	"gorm.io/gorm"

	domainreport "github.com/zkw/mini-schedule/backend/internal/domain/report"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// reportRepository 品牌看板聚合仓储（Batch 17）。纯读：N 条聚合 SQL，镜像 onboarding GetCounts。
// 所有查询都 join 到 class_sessions 别名 cs，故 data_scope 统一过滤 cs.location_id
// （location-less 表 holds/consumptions/waitlist/pending 经 NOT NULL FK join 到 cs，不改计数）。
type reportRepository struct {
	db *gorm.DB
}

// NewReportRepository 构造。
func NewReportRepository(db *gorm.DB) domainreport.Repository {
	return &reportRepository{db: db}
}

const reportSessionActiveStatuses = "('scheduled','in_progress','completed')"

// csLocFilter 返回针对 cs.location_id 的 scope/单门店过滤片段 + 参数（空串 = 全品牌不限）。
func csLocFilter(q domainreport.ReportQuery) (string, []any) {
	if q.LocationID != nil {
		return " AND cs.location_id = ?", []any{*q.LocationID}
	}
	if q.ScopeLocationIDs == nil {
		return "", nil
	}
	if len(q.ScopeLocationIDs) == 0 {
		return " AND 1 = 0", nil // 空 scope（店长无分配）→ 全 0
	}
	ph := make([]string, len(q.ScopeLocationIDs))
	args := make([]any, len(q.ScopeLocationIDs))
	for i, id := range q.ScopeLocationIDs {
		ph[i] = "?"
		args[i] = id
	}
	return " AND cs.location_id IN (" + strings.Join(ph, ",") + ")", args
}

func (r *reportRepository) BrandOverviewCounts(ctx context.Context, q domainreport.ReportQuery) (*domainreport.BrandOverview, error) {
	db := r.db.WithContext(ctx)
	loc, locArgs := csLocFilter(q)
	// 窗口查询参数：brand + [from,to) + scope。
	winArgs := func() []any { return append([]any{q.BrandID, q.From, q.To}, locArgs...) }
	out := &domainreport.BrandOverview{
		PopularCourses:       []domainreport.PopularCourse{},
		LocationDistribution: []domainreport.LocationDistribution{},
		InstructorSessions:   []domainreport.InstructorSessions{},
	}

	// A 组 1-4：一趟 FILTER 出 4 个计数（窗口内场次，按 booking status 分类）。
	var ab struct {
		BookingsTotal  int64
		AttendedTotal  int64
		CancelledTotal int64
		NoShowTotal    int64
	}
	if err := db.Raw(`SELECT
			COUNT(*) FILTER (WHERE b.status <> 'cancelled') AS bookings_total,
			COUNT(*) FILTER (WHERE b.status = 'attended')   AS attended_total,
			COUNT(*) FILTER (WHERE b.status = 'cancelled')  AS cancelled_total,
			COUNT(*) FILTER (WHERE b.status = 'no_show')    AS no_show_total
		FROM bookings b JOIN class_sessions cs ON cs.id = b.class_session_id
		WHERE cs.brand_id = ? AND cs.starts_at >= ? AND cs.starts_at < ?
		  AND cs.status IN `+reportSessionActiveStatuses+loc, winArgs()...).Scan(&ab).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合预约计数失败", err)
	}
	out.BookingsTotal, out.AttendedTotal, out.CancelledTotal, out.NoShowTotal = ab.BookingsTotal, ab.AttendedTotal, ab.CancelledTotal, ab.NoShowTotal

	// A 组 5：上座率 = 到课数(已完成场次) / SUM(已完成场次 capacity)。
	if err := db.Raw(`SELECT COALESCE(SUM(cs.capacity),0)
		FROM class_sessions cs
		WHERE cs.brand_id = ? AND cs.starts_at >= ? AND cs.starts_at < ? AND cs.status = 'completed'`+loc,
		winArgs()...).Scan(&out.TotalCapacity).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合容量失败", err)
	}
	if err := db.Raw(`SELECT COUNT(*)
		FROM bookings b JOIN class_sessions cs ON cs.id = b.class_session_id
		WHERE cs.brand_id = ? AND cs.starts_at >= ? AND cs.starts_at < ? AND cs.status = 'completed' AND b.status = 'attended'`+loc,
		winArgs()...).Scan(&out.AttendedInCompleted).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合到课数失败", err)
	}
	if out.TotalCapacity > 0 {
		out.OccupancyRate = float64(out.AttendedInCompleted) / float64(out.TotalCapacity)
	}

	// A 组 6：热门课程 Top5（按非取消预约数）。
	if err := db.Raw(`SELECT cs.course_id, c.title, COUNT(*) AS booking_count
		FROM bookings b JOIN class_sessions cs ON cs.id = b.class_session_id JOIN courses c ON c.id = cs.course_id
		WHERE cs.brand_id = ? AND cs.starts_at >= ? AND cs.starts_at < ? AND cs.status IN `+reportSessionActiveStatuses+`
		  AND b.status <> 'cancelled'`+loc+`
		GROUP BY cs.course_id, c.title ORDER BY booking_count DESC, cs.course_id LIMIT 5`,
		winArgs()...).Scan(&out.PopularCourses).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合热门课程失败", err)
	}

	// A 组 7：Location 分布（LEFT JOIN 保留 0 预约场次）。
	if err := db.Raw(`SELECT cs.location_id, l.name,
			COUNT(DISTINCT cs.id) AS session_count,
			COUNT(b.id) FILTER (WHERE b.status <> 'cancelled') AS booking_count
		FROM class_sessions cs JOIN locations l ON l.id = cs.location_id
		LEFT JOIN bookings b ON b.class_session_id = cs.id
		WHERE cs.brand_id = ? AND cs.starts_at >= ? AND cs.starts_at < ? AND cs.status IN `+reportSessionActiveStatuses+loc+`
		GROUP BY cs.location_id, l.name ORDER BY session_count DESC, cs.location_id`,
		winArgs()...).Scan(&out.LocationDistribution).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合 Location 分布失败", err)
	}

	// A 组 8：Instructor 授课场次数。
	if err := db.Raw(`SELECT cs.instructor_profile_id, ip.display_name AS name, COUNT(*) AS session_count
		FROM class_sessions cs JOIN instructor_profiles ip ON ip.id = cs.instructor_profile_id
		WHERE cs.brand_id = ? AND cs.starts_at >= ? AND cs.starts_at < ? AND cs.status IN `+reportSessionActiveStatuses+loc+`
		GROUP BY cs.instructor_profile_id, ip.display_name ORDER BY session_count DESC, cs.instructor_profile_id`,
		winArgs()...).Scan(&out.InstructorSessions).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合 Instructor 场次失败", err)
	}

	// B 组 9a：权益锁定次数（held_at 窗口内；scope 经 booking→session join）。
	if err := db.Raw(`SELECT COUNT(*)
		FROM entitlement_holds h JOIN bookings b ON b.id = h.booking_id JOIN class_sessions cs ON cs.id = b.class_session_id
		WHERE h.brand_id = ? AND h.held_at >= ? AND h.held_at < ?`+loc,
		winArgs()...).Scan(&out.EntitlementLockedTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合权益锁定失败", err)
	}
	// B 组 9b：权益消耗次数（consumed_at 窗口内）。
	if err := db.Raw(`SELECT COUNT(*)
		FROM entitlement_consumptions ec JOIN bookings b ON b.id = ec.booking_id JOIN class_sessions cs ON cs.id = b.class_session_id
		WHERE ec.brand_id = ? AND ec.consumed_at >= ? AND ec.consumed_at < ?`+loc,
		winArgs()...).Scan(&out.EntitlementConsumedTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合权益消耗失败", err)
	}

	// C 组 10/11：实时积压（忽略时间窗）。
	liveArgs := func() []any { return append([]any{q.BrandID}, locArgs...) }
	if err := db.Raw(`SELECT COUNT(*)
		FROM bookings b JOIN class_sessions cs ON cs.id = b.class_session_id
		WHERE cs.brand_id = ? AND b.status = 'pending_no_show'`+loc,
		liveArgs()...).Scan(&out.PendingNoShowTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合待处理爽约失败", err)
	}
	if err := db.Raw(`SELECT COUNT(*)
		FROM waitlist_entries w JOIN class_sessions cs ON cs.id = w.class_session_id
		WHERE w.brand_id = ? AND w.status IN ('waiting','eligible_to_promote')`+loc,
		liveArgs()...).Scan(&out.WaitlistTotal).Error; err != nil {
		return nil, apperr.ErrInternalF("聚合候补人数失败", err)
	}

	return out, nil
}
