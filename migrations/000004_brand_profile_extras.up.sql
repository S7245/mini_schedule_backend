-- 000004_brand_profile_extras.up.sql
-- Batch 4: 品牌资料补充字段 (description) — onboarding 第 1 步表单依赖。
ALTER TABLE brands
    ADD COLUMN IF NOT EXISTS description VARCHAR(2000);
