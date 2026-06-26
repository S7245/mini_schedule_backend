package commercial

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// ResourceKind 配额校验的资源类别。
type ResourceKind string

const (
	ResourceLocation ResourceKind = "location"
	ResourceStaff    ResourceKind = "staff"
	ResourceLearner  ResourceKind = "learner"
)

// SubscriptionGuard 在事务内做"锁可用 subscription → COUNT 资源 → 比 max"三段式。
//
// 调用方负责自己的 INSERT；guard 只做"锁 + 数 + 判"。
// 双窗口（grace_ends_at 优先于 expires_at）保留 Batch 4 review #2 的修复。
// Batch 16：可用态从 active 放宽为 active+grace_period（宽限期视同可用），
// 仍由时间门 grace_ends_at>now 把关；restricted/expired/frozen/cancelled 一律拦。
type SubscriptionGuard struct{}

// NewSubscriptionGuard 创建 guard（无状态，单例即可）。
func NewSubscriptionGuard() *SubscriptionGuard {
	return &SubscriptionGuard{}
}

// guardSubscriptionRow 是 SELECT FOR UPDATE 拿到的最小订阅信息。
// 避免依赖 persistence.BrandSubscriptionModel（拉到 application 层会引环）。
type guardSubscriptionRow struct {
	ID            int64
	BrandID       int64
	Status        string
	ExpiresAt     time.Time
	GraceEndsAt   *time.Time
	MaxLocations  int
	MaxStaffSeats int
	MaxLearners   int
}

// CheckAndCount 在外部传入的 tx 里执行：
//
//  1. SELECT FOR UPDATE 可用（active/grace_period）且未过期的 subscription
//  2. COUNT 当前 brand 下该 kind 的资源（排除软删，包含 inactive）
//  3. 超 max → 返 QUOTA_EXCEEDED + Details{current, max}
//     未超 → 返 (count, max, nil)
//
// 不可用的订阅一律 SUBSCRIPTION_RESTRICTED。
func (g *SubscriptionGuard) CheckAndCount(
	ctx context.Context,
	tx *gorm.DB,
	brandID int64,
	kind ResourceKind,
) (current, max int64, err error) {
	if tx == nil {
		return 0, 0, apperr.ErrInternalF("subscription_guard: nil tx", nil)
	}
	if brandID <= 0 {
		return 0, 0, apperr.NewAppError(apperr.ErrInvalidParam, "品牌 ID 无效", 400)
	}

	now := time.Now().UTC()
	var sub guardSubscriptionRow
	if err := tx.WithContext(ctx).
		Table("brand_subscriptions").
		Select("id, brand_id, status, expires_at, grace_ends_at, max_locations, max_staff_seats, max_learners").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("brand_id = ? AND status IN ? AND (grace_ends_at > ? OR (grace_ends_at IS NULL AND expires_at > ?))",
			brandID, []string{"active", "grace_period"}, now, now).
		Order("id DESC").
		First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, apperr.NewAppError(apperr.ErrSubscriptionRestricted, "未找到有效订阅", 403)
		}
		return 0, 0, apperr.ErrInternalF("查询订阅失败", err)
	}

	countSQL, countArgs, maxVal, err := g.dispatch(brandID, kind, sub)
	if err != nil {
		return 0, 0, err
	}

	var count int64
	if err := tx.WithContext(ctx).
		Raw(countSQL, countArgs...).
		Scan(&count).Error; err != nil {
		return 0, 0, apperr.ErrInternalF("统计资源数量失败", err)
	}

	if count >= maxVal {
		return count, maxVal, apperr.
			NewAppError(apperr.ErrQuotaExceeded, "已达套餐上限", 409).
			WithDetails(map[string]any{
				"current": count,
				"max":     maxVal,
				"kind":    string(kind),
			})
	}

	return count, maxVal, nil
}

// dispatch 按 kind 返回 COUNT SQL + max 字段。
func (g *SubscriptionGuard) dispatch(
	brandID int64,
	kind ResourceKind,
	sub guardSubscriptionRow,
) (sql string, args []interface{}, maxVal int64, err error) {
	switch kind {
	case ResourceLocation:
		return "SELECT COUNT(*) FROM locations WHERE brand_id = ? AND deleted_at IS NULL",
			[]interface{}{brandID},
			int64(sub.MaxLocations),
			nil

	case ResourceStaff:
		// staff = brand_users（含 owner）；deleted_at IS NULL（软删过滤）。
		// is_owner 也算席位（一个品牌至少 1 个 active owner）。
		return "SELECT COUNT(*) FROM brand_users WHERE brand_id = ? AND deleted_at IS NULL",
			[]interface{}{brandID},
			int64(sub.MaxStaffSeats),
			nil

	case ResourceLearner:
		// brand_learner_profiles 是 Batch 7+ 的 Learner 资源（per 蓝图）。
		// 表结构已在 migration 000003 里就位。
		return "SELECT COUNT(*) FROM brand_learner_profiles WHERE brand_id = ? AND deleted_at IS NULL",
			[]interface{}{brandID},
			int64(sub.MaxLearners),
			nil

	default:
		return "", nil, 0, fmt.Errorf("subscription_guard: unknown kind %q", kind)
	}
}
