-- 000010_learner_profile_softdelete_unique.down.sql
--
-- 回滚为 000003 的无条件唯一索引。注意：若库中已存在「同 (brand_id, learner_identity_id) 的
-- 一条软删行 + 一条 active 行」（000010 后正常产生的重新建档场景），此 CREATE 会失败——
-- 这是回滚此类 partial→full 变更的固有风险，需先清理重复软删行再降级。

DROP INDEX IF EXISTS idx_brand_learner_profiles_brand_identity;
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_learner_profiles_brand_identity
    ON brand_learner_profiles(brand_id, learner_identity_id);
