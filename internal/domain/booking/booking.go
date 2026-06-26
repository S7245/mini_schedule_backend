// Package booking 预约下单领域（bookings / entitlement_holds，Batch 13c）。
//
// 本批做 brand 后台 staff_assisted 下单 + 代取消 + 场次取消级联 + 预约规则解析。
// 下单原子性：单事务内先锁 class_sessions 行（校窗口/容量/booked_count++），再锁
// learner_entitlements 行（按 §5.7 选权益/锁额/建 hold/流水）；行锁 + unique + CHECK 兜超卖。
// 候补（waitlist）留 13d；签到/consume/no_show 留 13e；C 端自助预约留 Batch 14。
package booking

import (
	"context"
	"sort"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
)

// Status 预约状态机（与 DB CHECK bookings_status_valid 对齐）。
// 本批只产生 booked / cancelled；attended/pending_no_show/no_show 留 13e。
type Status string

const (
	StatusBooked        Status = "booked"
	StatusCancelled     Status = "cancelled"
	StatusAttended      Status = "attended"
	StatusPendingNoShow Status = "pending_no_show"
	StatusNoShow        Status = "no_show"
)

// IsValidStatus 判断输入是否合法预约状态。
func IsValidStatus(s string) bool {
	switch Status(s) {
	case StatusBooked, StatusCancelled, StatusAttended, StatusPendingNoShow, StatusNoShow:
		return true
	}
	return false
}

// CanAttend 报告该预约状态可否签到（标到课，Batch 13e）。booked 为正常签到；
// pending_no_show 为「结束场次后补签纠错」——学员实际到场但员工晚标，pending_no_show→attended
// 天然纠错，免做撤销误签到（§20.12 撤销留人工权益调整）。
func CanAttend(s Status) bool {
	return s == StatusBooked || s == StatusPendingNoShow
}

// CanConfirmNoShow 报告该预约状态可否确认爽约（Batch 13e）。仅 pending_no_show（须先经「结束场次」）。
func CanConfirmNoShow(s Status) bool {
	return s == StatusPendingNoShow
}

// Source 预约来源（与 DB CHECK bookings_source_valid 对齐）。
// 本批只产生 staff_assisted；learner_self_service 留 Batch 14、waitlist_promotion 留 13d。
type Source string

const (
	SourceLearnerSelfService Source = "learner_self_service"
	SourceStaffAssisted      Source = "staff_assisted"
	SourceWaitlistPromotion  Source = "waitlist_promotion"
)

// CancelSource 取消来源（与 DB CHECK bookings_cancel_source_valid 对齐）。
const (
	CancelSourceLearner          = "learner"
	CancelSourceStaff            = "staff"
	CancelSourceSessionCancelled = "session_cancelled"
	CancelSourceSystem           = "system"
)

// HoldStatus 权益锁定状态（与 DB CHECK entitlement_holds_status_valid 对齐）。
type HoldStatus string

const (
	HoldStatusHeld     HoldStatus = "held"
	HoldStatusReleased HoldStatus = "released"
	HoldStatusConsumed HoldStatus = "consumed"
)

// RecordType session_records.record_type（与 DB CHECK session_records_type_valid 对齐，Batch 13e）。
type RecordType string

const (
	RecordAttendance RecordType = "attendance"
	RecordNoShow     RecordType = "no_show"
	RecordManual     RecordType = "manual"
)

// ConsumptionType entitlement_consumptions.consumption_type（与 DB CHECK 对齐，Batch 13e）。
type ConsumptionType string

const (
	ConsumptionAttendance ConsumptionType = "attendance"
	ConsumptionNoShow     ConsumptionType = "no_show"
	ConsumptionManual     ConsumptionType = "manual"
)

// EntitlementMode 代预约的权益选择模式（请求级，非落库枚举）。
type EntitlementMode string

const (
	// ModeAuto 系统按 §5.7 优先级自动选权益。
	ModeAuto EntitlementMode = "auto"
	// ModeManual 员工手动指定具体权益（仍全量校验，非法报错不回退）。
	ModeManual EntitlementMode = "manual"
	// ModeNone 无权益占位（仅 booking.create_assisted，须填原因，不绕容量，不建 hold）。
	ModeNone EntitlementMode = "none"
)

// IsValidEntitlementMode 判断输入是否合法权益模式。
func IsValidEntitlementMode(s string) bool {
	switch EntitlementMode(s) {
	case ModeAuto, ModeManual, ModeNone:
		return true
	}
	return false
}

// ---- 预约规则解析（§11）----

// Policy 一条完整预约策略（brand_booking_policies 行；所有字段有具体值）。
// BookAheadMaxMinutes / DailyBookingLimit / WeeklyBookingLimit / ConcurrentBookingLimit
// 为 nil 表示「不限」。注意：brand_booking_policies 无 monthly 列，月限独家来自权益产品。
type Policy struct {
	BookAheadMinMinutes       int  `json:"book_ahead_min_minutes"`
	BookAheadMaxMinutes       *int `json:"book_ahead_max_minutes"`
	CancelDeadlineMinutes     int  `json:"cancel_deadline_minutes"`
	ReleaseOnCancel           bool `json:"release_on_cancel"`
	NoShowConsumesEntitlement bool `json:"no_show_consumes_entitlement"`
	DailyBookingLimit         *int `json:"daily_booking_limit"`
	WeeklyBookingLimit        *int `json:"weekly_booking_limit"`
	ConcurrentBookingLimit    *int `json:"concurrent_booking_limit"`
	AllowWaitlist             bool `json:"allow_waitlist"`
	WaitlistLimit             int  `json:"waitlist_limit"`
}

// PolicyOverride 场次覆盖（class_session_policy_overrides；nil 字段 = 继承 base）。
// 注意：override 有 AllowCancel（brand 层无此字段，brand 默认恒允许取消），无 book_ahead。
type PolicyOverride struct {
	AllowCancel               *bool
	CancelDeadlineMinutes     *int
	ReleaseOnCancel           *bool
	NoShowConsumesEntitlement *bool
	AllowWaitlist             *bool
	WaitlistLimit             *int
}

// EffectivePolicy base 叠 override 后的生效策略。AllowCancel 在 brand 层恒 true，override 可关。
type EffectivePolicy struct {
	Policy
	AllowCancel bool
}

// DefaultPolicy 无任何 policy 行时的 sensible 默认（窗口不限、截止 0、取消退权益、不扣爽约、频次不限）。
func DefaultPolicy() Policy {
	return Policy{
		BookAheadMinMinutes:       0,
		BookAheadMaxMinutes:       nil,
		CancelDeadlineMinutes:     0,
		ReleaseOnCancel:           true,
		NoShowConsumesEntitlement: false,
		DailyBookingLimit:         nil,
		WeeklyBookingLimit:        nil,
		ConcurrentBookingLimit:    nil,
		AllowWaitlist:             true,
		WaitlistLimit:             0,
	}
}

// ResolveEffectivePolicy 把 base（location 行或 brand-default 行或 DefaultPolicy）叠加场次稀疏 override。
func ResolveEffectivePolicy(base Policy, ov *PolicyOverride) EffectivePolicy {
	eff := EffectivePolicy{Policy: base, AllowCancel: true}
	if ov == nil {
		return eff
	}
	if ov.AllowCancel != nil {
		eff.AllowCancel = *ov.AllowCancel
	}
	if ov.CancelDeadlineMinutes != nil {
		eff.CancelDeadlineMinutes = *ov.CancelDeadlineMinutes
	}
	if ov.ReleaseOnCancel != nil {
		eff.ReleaseOnCancel = *ov.ReleaseOnCancel
	}
	if ov.NoShowConsumesEntitlement != nil {
		eff.NoShowConsumesEntitlement = *ov.NoShowConsumesEntitlement
	}
	if ov.AllowWaitlist != nil {
		eff.AllowWaitlist = *ov.AllowWaitlist
	}
	if ov.WaitlistLimit != nil {
		eff.WaitlistLimit = *ov.WaitlistLimit
	}
	return eff
}

// WithinBookingWindow 预约窗口校验：now 须落在 [starts-max, starts-min]。
// min = 至少提前多少分钟（now 须 ≤ starts-min）；max = 最多提前多少分钟（now 须 ≥ starts-max，nil=不限）。
func WithinBookingWindow(now, startsAt time.Time, p EffectivePolicy) bool {
	latest := startsAt.Add(-time.Duration(p.BookAheadMinMinutes) * time.Minute)
	if now.After(latest) {
		return false
	}
	if p.BookAheadMaxMinutes != nil {
		earliest := startsAt.Add(-time.Duration(*p.BookAheadMaxMinutes) * time.Minute)
		if now.Before(earliest) {
			return false
		}
	}
	return true
}

// CancelDeadlinePassed 取消截止校验：now 晚于 starts-cancel_deadline 则已过截止。
func CancelDeadlinePassed(now, startsAt time.Time, p EffectivePolicy) bool {
	deadline := startsAt.Add(-time.Duration(p.CancelDeadlineMinutes) * time.Minute)
	return now.After(deadline)
}

// ---- 权益自动选择（§5.7）----

// EntitlementCandidate 已通过「可用性」过滤（active/未过期/scope 匹配/频次/余额）的候选权益，
// 仅携带 §5.7 排序所需字段。
type EntitlementCandidate struct {
	EntitlementID int64
	ProductType   entitlement.ProductType
	// CourseSpecific 该权益产品 course_scope=specific 且命中本场次课程（指定课程权益优先）。
	CourseSpecific bool
	ExpiresAt      time.Time
}

// typeRank 次数/课时包（class/trial）优先于会员卡（§5.7 规则 3）。
func typeRank(t entitlement.ProductType) int {
	if t == entitlement.ProductMembershipCard {
		return 1
	}
	return 0
}

// SortCandidates 按 §5.7 优先级原地排序：① 指定课程权益优先 ② 最早过期 ③ 课时包先于会员卡 ④ id 稳定。
// 排序后 [0] 即自动选中项；usable-entitlements 端点也复用此序展示。
func SortCandidates(cands []EntitlementCandidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.CourseSpecific != b.CourseSpecific {
			return a.CourseSpecific // true 在前
		}
		if !a.ExpiresAt.Equal(b.ExpiresAt) {
			return a.ExpiresAt.Before(b.ExpiresAt)
		}
		if ra, rb := typeRank(a.ProductType), typeRank(b.ProductType); ra != rb {
			return ra < rb
		}
		return a.EntitlementID < b.EntitlementID
	})
}

// SelectAuto 返回 §5.7 自动选中的候选（候选空时 ok=false）。
func SelectAuto(cands []EntitlementCandidate) (EntitlementCandidate, bool) {
	if len(cands) == 0 {
		return EntitlementCandidate{}, false
	}
	SortCandidates(cands)
	return cands[0], true
}

// ---- 实体 ----

// Hold 预约绑定的权益锁（占位预约无 hold，为 nil）。
type Hold struct {
	ID                   int64      `json:"id"`
	LearnerEntitlementID int64      `json:"learner_entitlement_id"`
	ProductName          string     `json:"product_name"`
	Status               HoldStatus `json:"status"`
	Credits              int        `json:"credits"`
}

// Booking 预约实体（含列表/详情用的反范式快照）。
type Booking struct {
	ID                     int64      `json:"id"`
	BrandID                int64      `json:"brand_id"`
	ClassSessionID         int64      `json:"class_session_id"`
	BrandLearnerProfileID  int64      `json:"brand_learner_profile_id"`
	Source                 Source     `json:"source"`
	Status                 Status     `json:"status"`
	BookedAt               time.Time  `json:"booked_at"`
	CancelledAt            *time.Time `json:"cancelled_at"`
	CancelledBy            *int64     `json:"cancelled_by"`
	CancelSource           string     `json:"cancel_source"`
	CancelReason           string     `json:"cancel_reason"`
	AssistedBy             *int64     `json:"assisted_by"`
	RequiresEntitlementFix bool       `json:"requires_entitlement_fix"`
	NoEntitlementReason    string     `json:"no_entitlement_reason"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	// 反范式（JOIN 取）。
	SessionStartsAt time.Time `json:"session_starts_at"`
	SessionEndsAt   time.Time `json:"session_ends_at"`
	SessionStatus   string    `json:"session_status"`
	CourseTitle     string    `json:"course_title"`
	LocationID      int64     `json:"location_id"`
	LocationName    string    `json:"location_name"`
	LearnerName     string    `json:"learner_name"`
	LearnerPhone    string    `json:"learner_phone"`
	Hold            *Hold     `json:"hold"`
}

// UsableEntitlement usable-entitlements 端点的候选权益（喂自动预览 + 手动下拉）。
type UsableEntitlement struct {
	EntitlementID    int64                   `json:"entitlement_id"`
	ProductName      string                  `json:"product_name"`
	ProductType      entitlement.ProductType `json:"product_type"`
	RemainingCredits *int                    `json:"remaining_credits"`
	ExpiresAt        time.Time               `json:"expires_at"`
	AutoSelected     bool                    `json:"auto_selected"`
}

// EndSessionResult 结束场次结果（Batch 13e）：场次→completed + 未签到 booked 批量→pending_no_show。
type EndSessionResult struct {
	SessionID          int64  `json:"session_id"`
	Status             string `json:"status"`
	PendingNoShowCount int    `json:"pending_no_show_count"`
}

// CreateInput 代预约入参。
type CreateInput struct {
	BrandID               int64
	ActorID               int64
	ClassSessionID        int64
	BrandLearnerProfileID int64
	EntitlementMode       EntitlementMode
	LearnerEntitlementID  *int64 // ModeManual 必填
	NoEntitlementReason   string // ModeNone 必填
	ScopeLocationIDs      []int64
}

// LearnerCreateInput C 端学员自助下单入参（Batch 14a）。无 ActorID（assisted_by 落 NULL）、无
// ScopeLocationIDs（学员无 data_scope）；source 恒 learner_self_service、mode 恒 auto（§5.7 学员不自选权益）。
type LearnerCreateInput struct {
	BrandID               int64
	ClassSessionID        int64
	BrandLearnerProfileID int64
}

// ListFilter 预约列表查询。零值不过滤；ScopeLocationIDs 非 nil 时按 data_scope 收紧。
type ListFilter struct {
	BrandID               int64
	ClassSessionID        int64
	LocationID            int64
	BrandLearnerProfileID int64
	Status                string
	// Statuses 多状态 IN 过滤（Batch 14b 上课记录 attended+no_show）。非空时优先于单 Status。
	Statuses               []string
	RequiresEntitlementFix *bool
	ScopeLocationIDs       []int64
}

// Repository 预约仓储接口。
type Repository interface {
	// Create 单事务 TX-1：锁 session→校窗口/容量→（auto/manual 锁权益+hold+流水 / none 占位）。
	Create(ctx context.Context, in CreateInput) (*Booking, error)
	// CreateByLearner 单事务 TX-L1（Batch 14a 学员自助）：锁 session(scheduled/窗口)→跨场次时间重叠校验
	// （§22.1，重叠→BOOKING_TIME_CONFLICT）→placeBooking(auto/source=learner_self_service/assisted=nil)→audit(learner)。
	// 无 data_scope（ScopeLocationIDs=nil）；前端只读课程表已限 scheduled。
	CreateByLearner(ctx context.Context, in LearnerCreateInput) (*Booking, error)
	// Cancel 单事务 TX-2：锁 session+booking→校 deadline/allow_cancel→cancelled+退名额+release/forfeit hold。
	Cancel(ctx context.Context, brandID, actorID, id int64, reason string) (*Booking, error)
	// CancelByLearner 单事务 TX-L2（Batch 14a 学员自助）：tx 内校所有权（booking.profile==profileID，否则
	// BOOKING_NOT_FOUND 404 不泄漏）→锁 session+booking→校 deadline/allow_cancel→cancelled(cancel_source=learner,
	// cancelled_by NULL)+退名额+release/forfeit hold。
	CancelByLearner(ctx context.Context, brandID, profileID, id int64, reason string) (*Booking, error)
	// Attend 单事务 TX-A（Batch 13e）：锁 booking+session→booked|pending_no_show→attended→attendance_records→
	// hold consume（held→consumed + entitlement_consumptions + session_records + txn consume）。占位无 hold→不消费。
	// data_scope 守卫由 service 层完成（GetByID + guardLocationInScope）。
	Attend(ctx context.Context, brandID, actorID, id int64, note string) (*Booking, error)
	// EndSession 单事务 TX-B（Batch 13e）：锁 session→校 scheduled/in_progress→completed→未签到 booked 批量→pending_no_show。
	// scopeLocationIDs 非 nil 时守卫场次门店（越权 SESSION_NOT_FOUND）。
	EndSession(ctx context.Context, brandID, actorID, sessionID int64, scopeLocationIDs []int64) (*EndSessionResult, error)
	// EndSessionSystem 单事务（Batch 15 自动化）：系统版结束场次。按 id 锁 session（跨品牌、无 scope），
	// 从行读 brand_id，复用 EndSession 的 applyEndSession 核心（actor=system，audit actor_id NULL）。
	// 幂等：completed 场次返 SESSION_NOT_ENDABLE。§22.6：只产 pending_no_show，绝不自动 no_show/扣课。
	EndSessionSystem(ctx context.Context, sessionID int64) (*EndSessionResult, error)
	// MarkSessionsInProgress 批量 scheduled→in_progress（Batch 15 自动化）：
	// status='scheduled' AND starts_at <= now AND ends_at > now。纯显示态、无 audit、幂等。返回受影响行数。
	MarkSessionsInProgress(ctx context.Context, now time.Time) (int64, error)
	// ListDueSessionIDs 列「到点未结束」场次 id（Batch 15 自动化）：
	// status IN (scheduled,in_progress) AND ends_at <= now。跨品牌（系统全局扫描）。
	ListDueSessionIDs(ctx context.Context, now time.Time) ([]int64, error)
	// ConfirmNoShow 单事务 TX-C（Batch 13e）：锁 booking→校 pending_no_show→no_show→按 policy
	// no_show_consumes_entitlement consume/release hold + session_records(no_show)。data_scope 守卫由 service 完成。
	ConfirmNoShow(ctx context.Context, brandID, actorID, id int64, reason string) (*Booking, error)
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Booking, int64, error)
	GetByID(ctx context.Context, brandID, id int64) (*Booking, error)
	// UsableEntitlements 返回某学员对某场次的可用权益（§5.7 序，[0].AutoSelected=true）。
	// scopeLocationIDs 非 nil 时按 data_scope 守卫场次（越权 → SESSION_NOT_FOUND）。
	UsableEntitlements(ctx context.Context, brandID, sessionID, learnerID int64, scopeLocationIDs []int64) ([]*UsableEntitlement, error)

	// GetDefaultPolicy 读 brand-default 策略行（location_id IS NULL）；无行返 DefaultPolicy。
	GetDefaultPolicy(ctx context.Context, brandID int64) (*Policy, error)
	// UpsertDefaultPolicy upsert brand-default 行 + audit。
	UpsertDefaultPolicy(ctx context.Context, brandID, actorID int64, p Policy) (*Policy, error)
}
