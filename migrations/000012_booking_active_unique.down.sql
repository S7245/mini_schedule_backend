-- 000012_booking_active_unique.down.sql
--
-- 回滚为 000003 的无条件全量唯一索引。注意：若库中已存在「同 (class_session_id,
-- brand_learner_profile_id) 的一条 cancelled 行 + 一条 active 行」（000012 后正常产生的重约
-- 场景），此 CREATE 会失败——partial→full 回滚的固有风险，需先清理重复 cancelled 行再降级。

DROP INDEX IF EXISTS idx_bookings_session_learner_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_session_learner_unique
    ON bookings(class_session_id, brand_learner_profile_id);

DROP INDEX IF EXISTS idx_waitlist_entries_session_learner_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlist_entries_session_learner_unique
    ON waitlist_entries(class_session_id, brand_learner_profile_id);
