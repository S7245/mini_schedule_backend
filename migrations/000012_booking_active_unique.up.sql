-- 000012_booking_active_unique.up.sql
--
-- Batch 13c：预约下单落地。
--
-- 000003 的 idx_bookings_session_learner_unique(class_session_id, brand_learner_profile_id)
-- 是无条件全量唯一。后果：学员对某场次取消预约后，想重新预约同一场次会撞这条 cancelled
-- 死行 → 误报 BOOKING_DUPLICATE，永久无法重约（镜像 13a learner brand_identity 的 000010 修法）。
--
-- 改成仅对「未取消」预约唯一：cancelled 行被排除，重约时 INSERT 新行（新 id），旧 cancelled
-- 历史保留可审计。attended/pending_no_show/no_show 仍占唯一槽（场次已发生，不应重约）。
--
-- waitlist_entries 同款修法（为 13d 候补铺路，本批不写 waitlist，幂等无害）：仅对仍在队
-- （waiting/eligible_to_promote/promoted）唯一，cancelled/skipped 后可重新加入候补。
--
-- 无权限变更：booking.view/create_assisted/cancel 三码及全角色映射在 000003 base 已 seed 齐
-- 且存量 brand 建库即 copy 进 brand_role_permissions（非 13a/13b 后补码），无需 backfill。

DROP INDEX IF EXISTS idx_bookings_session_learner_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_session_learner_active
    ON bookings(class_session_id, brand_learner_profile_id)
    WHERE status <> 'cancelled';

DROP INDEX IF EXISTS idx_waitlist_entries_session_learner_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlist_entries_session_learner_active
    ON waitlist_entries(class_session_id, brand_learner_profile_id)
    WHERE status NOT IN ('cancelled', 'skipped');
