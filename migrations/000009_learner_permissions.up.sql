-- 000009_learner_permissions.up.sql
--
-- Batch 13a — 学员档案管理（BrandLearnerProfile + 标签）。
--
-- 学员/预约相关表（brand_learner_profiles / learner_identities / learner_tags /
-- learner_tag_assignments 等）已在 000003 建好，本批不动表结构，仅补 learner 细粒度
-- 写权限 seed（镜像 000005 给 location 补 create/edit/delete 的做法）。
--
-- 现状：000003 已 seed 粗粒度 learner.view + learner.manage 并映射到全部模板。learner.view
-- 沿用作读权限门（已映射到所有角色，本批不再追加映射）；learner.manage 粗码保留但 service 层
-- 不再校验（成为 inert 码，同 location.manage 的处置）。本批新增 4 个细粒度写码，service 实际校验。
--
-- 角色→权限映射（按 blueprint §21.1/§21.2，学员写操作仅 brand_owner / brand_admin；
-- course_operator / finance_support / location_manager / receptionist / instructor /
-- location_assistant 仅 learner.view，已由 000003 映射）：
--   brand_owner / brand_admin：learner.create / edit / delete / freeze 全 4 码。
-- 注：000003 曾给 location_manager / receptionist 粗码 learner.manage（write），但 §21 明确
--   location 级角色对学员仅「查看」，故本批的细粒度写码不映射给它们；遗留的 learner.manage
--   成为 inert，不影响 service 校验（service 只认 learner.create/edit/delete/freeze）。

-- 1) 细粒度写权限（learner.view 已存在于 000003，不重复 INSERT）。
INSERT INTO permissions (code, domain, action, name, description) VALUES
    ('learner.create', 'learner', 'create', '新增学员', '创建 BrandLearnerProfile'),
    ('learner.edit',   'learner', 'edit',   '编辑学员', '编辑学员资料 / 标签'),
    ('learner.delete', 'learner', 'delete', '删除学员', '软删 BrandLearnerProfile'),
    ('learner.freeze', 'learner', 'freeze', '冻结学员', '冻结 / 解冻学员')
ON CONFLICT (code) DO NOTHING;

-- 2) role_template 权限映射（仅 brand_owner / brand_admin 写）。
WITH mappings(template_code, permission_code) AS (
    VALUES
        ('brand_owner', 'learner.create'),
        ('brand_owner', 'learner.edit'),
        ('brand_owner', 'learner.delete'),
        ('brand_owner', 'learner.freeze'),
        ('brand_admin', 'learner.create'),
        ('brand_admin', 'learner.edit'),
        ('brand_admin', 'learner.delete'),
        ('brand_admin', 'learner.freeze')
)
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM mappings m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (template_id, permission_id) DO NOTHING;

-- 3) BACKFILL 存量 brand：对每个 brand 的 brand_owner / brand_admin system role 注入新写码。
--    通过 role_template_permissions JOIN，复用 step 2 刚写好的模板映射（镜像 000008）。
INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
SELECT br.brand_id, br.id, rtp.permission_id
FROM brand_roles br
JOIN role_templates rt ON rt.id = br.template_id
JOIN role_template_permissions rtp ON rtp.template_id = rt.id
JOIN permissions p ON p.id = rtp.permission_id
WHERE p.code IN ('learner.create', 'learner.edit', 'learner.delete', 'learner.freeze')
ON CONFLICT (role_id, permission_id) DO NOTHING;
