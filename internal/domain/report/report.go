// Package report 品牌基础运营看板的读模型 + 聚合仓储接口（Batch 17，§15 品牌看板）。
//
// 纯读聚合：一次调用出全部 11 指标。无业务状态机、无写、无 audit。
package report

import (
	"context"
	"time"
)

// ReportQuery 品牌看板查询入参（时间窗 + data_scope 已由 application 层解析）。
type ReportQuery struct {
	BrandID int64
	// ScopeLocationIDs：nil = 全品牌（owner/admin/course_operator）；
	// 非 nil = 仅这些 location（店长 assigned）；空切片 = 拒绝所有（→ 全 0）。
	ScopeLocationIDs []int64
	// LocationID：可选，进一步限定单门店（application 层已校验 ∈ scope）。
	LocationID *int64
	From       time.Time // 窗口左闭
	To         time.Time // 窗口右开
}

// PopularCourse 热门课程一行（按预约数排序）。
type PopularCourse struct {
	CourseID     int64  `json:"course_id"`
	Title        string `json:"title"`
	BookingCount int64  `json:"booking_count"`
}

// LocationDistribution Location 场次/预约分布一行。
type LocationDistribution struct {
	LocationID   int64  `json:"location_id"`
	Name         string `json:"name"`
	SessionCount int64  `json:"session_count"`
	BookingCount int64  `json:"booking_count"`
}

// InstructorSessions Instructor 授课场次一行。
type InstructorSessions struct {
	InstructorProfileID int64  `json:"instructor_profile_id"`
	Name                string `json:"name"`
	SessionCount        int64  `json:"session_count"`
}

// BrandOverview 品牌运营看板一次聚合的全部 11 指标。
//
// A 组（周期活动，锚定场次 starts_at）：Bookings/Attended/Cancelled/NoShow/Occupancy/
//
//	PopularCourses/LocationDistribution/InstructorSessions。
//
// B 组（周期事件，锚定事件时间戳）：EntitlementLocked(held_at)/EntitlementConsumed(consumed_at)。
// C 组（实时积压，忽略时间窗）：PendingNoShow/Waitlist。
type BrandOverview struct {
	BookingsTotal            int64                  `json:"bookings_total"`
	AttendedTotal            int64                  `json:"attended_total"`
	CancelledTotal           int64                  `json:"cancelled_total"`
	NoShowTotal              int64                  `json:"no_show_total"`
	OccupancyRate            float64                `json:"occupancy_rate"`
	TotalCapacity            int64                  `json:"total_capacity"`
	AttendedInCompleted      int64                  `json:"attended_in_completed"`
	EntitlementLockedTotal   int64                  `json:"entitlement_locked_total"`
	EntitlementConsumedTotal int64                  `json:"entitlement_consumed_total"`
	PendingNoShowTotal       int64                  `json:"pending_no_show_total"`
	WaitlistTotal            int64                  `json:"waitlist_total"`
	PopularCourses           []PopularCourse        `json:"popular_courses"`
	LocationDistribution     []LocationDistribution `json:"location_distribution"`
	InstructorSessions       []InstructorSessions   `json:"instructor_sessions"`
}

// Repository 品牌看板聚合仓储。
type Repository interface {
	BrandOverviewCounts(ctx context.Context, q ReportQuery) (*BrandOverview, error)
}
