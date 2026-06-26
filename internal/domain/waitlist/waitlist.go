// Package waitlist 候补领域（waitlist_entries 表，Batch 13d）。
//
// 简单候补（§12/§22.4）：场次满员后学员加入候补（不锁权益）；员工手动转正（校容量+锁权益，
// 创建 source=waitlist_promotion 的 booking）；员工可跳过(skipped)或取消(cancelled)。
// 不做自动通知/限时确认/超时顺延/候补时锁权益（后续池）；C 端自助候补留 Batch 14。
package waitlist

import (
	"context"
	"time"
)

// Status 候补状态（与 DB CHECK waitlist_entries_status_valid 对齐）。
// 本批用 waiting→{promoted,skipped,cancelled}；eligible_to_promote 保留枚举（决策1 手动转正+容量门，不主动写）。
type Status string

const (
	StatusWaiting           Status = "waiting"
	StatusEligibleToPromote Status = "eligible_to_promote"
	StatusPromoted          Status = "promoted"
	StatusCancelled         Status = "cancelled"
	StatusSkipped           Status = "skipped"
)

// IsValidStatus 判断输入是否合法候补状态。
func IsValidStatus(s string) bool {
	switch Status(s) {
	case StatusWaiting, StatusEligibleToPromote, StatusPromoted, StatusCancelled, StatusSkipped:
		return true
	}
	return false
}

// Entry 候补实体（含列表/drawer 用的反范式快照）。
type Entry struct {
	ID                    int64     `json:"id"`
	BrandID               int64     `json:"brand_id"`
	ClassSessionID        int64     `json:"class_session_id"`
	BrandLearnerProfileID int64     `json:"brand_learner_profile_id"`
	Position              int       `json:"position"`
	Status                Status    `json:"status"`
	PromotedBookingID     *int64    `json:"promoted_booking_id"`
	SkippedReason         string    `json:"skipped_reason"`
	OperatedBy            *int64    `json:"operated_by"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	// 反范式（JOIN 取）。
	LearnerName     string    `json:"learner_name"`
	LearnerPhone    string    `json:"learner_phone"`
	SessionStartsAt time.Time `json:"session_starts_at"`
	CourseTitle     string    `json:"course_title"`
	LocationID      int64     `json:"location_id"`
	LocationName    string    `json:"location_name"`
}

// JoinInput 加入候补入参。ScopeLocationIDs 非 nil 时 repo 内按 data_scope 守卫场次（越权 404）。
type JoinInput struct {
	BrandID               int64
	ActorID               int64
	ClassSessionID        int64
	BrandLearnerProfileID int64
	ScopeLocationIDs      []int64
	// SelfService C 端学员自助加入（Batch 14b）：operated_by 落 NULL（brand_users FK，学员非 brand_user）、
	// audit actor=learner（actor_id=BrandLearnerProfileID）。默认 false=staff 代加入（operated_by=ActorID）。
	SelfService bool
}

// PromoteInput 转正入参（权益模式复用 13c：auto/manual/none 占位）。data_scope 由 service GetByID 守卫。
type PromoteInput struct {
	BrandID              int64
	ActorID              int64
	EntryID              int64
	EntitlementMode      string
	LearnerEntitlementID *int64
	NoEntitlementReason  string
}

// Repository 候补仓储接口。
type Repository interface {
	// Join 锁 session（串行化 position）→ 校满员/allow_waitlist/waitlist_limit/已约/重复 → INSERT waiting（不锁权益）。
	Join(ctx context.Context, in JoinInput) (*Entry, error)
	// ListBySession 按 position 列该场次候补（scopeLocationIDs 非 nil 时守卫场次门店）。
	ListBySession(ctx context.Context, brandID, sessionID int64, scopeLocationIDs []int64) ([]*Entry, error)
	// ListByLearner C 端「我的候补」（Batch 14b）：本 profile 活跃(waiting/eligible)候补，按场次时间序。
	ListByLearner(ctx context.Context, brandID, profileID int64) ([]*Entry, error)
	// CancelByLearner C 端自助取消候补（Batch 14b）：tx 内校所有权（越权 WAITLIST_ENTRY_NOT_FOUND 404）+
	// waiting/eligible→cancelled + operated_by NULL + audit actor=learner。
	CancelByLearner(ctx context.Context, brandID, profileID, id int64) (*Entry, error)
	GetByID(ctx context.Context, brandID, id int64) (*Entry, error)
	// Promote 锁 session+entry → 校 waiting + 容量 → placeBooking(waitlist_promotion) → entry promoted+promoted_booking_id。
	Promote(ctx context.Context, in PromoteInput) (*Entry, error)
	// Skip 校 waiting → skipped + reason。
	Skip(ctx context.Context, brandID, actorID, id int64, reason string) (*Entry, error)
	// Cancel 校 waiting/eligible → cancelled。
	Cancel(ctx context.Context, brandID, actorID, id int64) (*Entry, error)
}
