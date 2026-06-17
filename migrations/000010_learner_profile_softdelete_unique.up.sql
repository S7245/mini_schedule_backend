-- 000010_learner_profile_softdelete_unique.up.sql
--
-- Batch 13a code-review P1 修复。
--
-- 000003 的 idx_brand_learner_profiles_brand_identity 是无条件唯一（brand_id, learner_identity_id），
-- 缺 `WHERE deleted_at IS NULL`——与同表的 idx_brand_learner_profiles_brand_learner_no（partial）
-- 及 locations 等所有软删表的唯一索引惯例不一致。后果：软删一个学员后，再用同手机号（→同 identity）
-- 在本品牌新建会撞这条死行 → 误报 LEARNER_ALREADY_EXISTS，永久无法重新建档。
--
-- 改成仅对未软删行唯一：软删行的 deleted_at 非空被排除，重新建档插入新行（新 id）即可，
-- 与门店/学号 re-create 语义一致。

DROP INDEX IF EXISTS idx_brand_learner_profiles_brand_identity;
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_learner_profiles_brand_identity
    ON brand_learner_profiles(brand_id, learner_identity_id)
    WHERE deleted_at IS NULL;
