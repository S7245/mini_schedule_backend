-- 000011_entitlement_role_mappings.up.sql
--
-- Batch 13b — 权益产品 + 发放。
--
-- 000003 已 seed 粗粒度 entitlement.view / entitlement.manage / entitlement.adjust 三码，
-- 但模板映射不全，对不上 blueprint §21.1（权益/会员卡：品牌负责人=全部、品牌管理员=全部、
-- 课程运营=查看、财务/售后=全部）。本批不加新码、不动表，仅补 3 条缺失映射 + 存量 backfill：
--   brand_admin    += entitlement.adjust   （§21.1 品牌管理员=全部，000003 漏 adjust）
--   course_operator += entitlement.view     （§21.1 课程运营=查看，000003 完全没给）
--   finance_support += entitlement.adjust   （§21.1 财务/售后=全部，000003 漏 adjust）
-- service 层校验：manage=产品模板 CRUD + 发放；adjust=对已发放权益的额度/状态手动干预；view=只读。

-- 1) 补 role_template 映射（仅 3 条；其余 entitlement.* 映射 000003 已建）。
WITH new_maps(template_code, permission_code) AS (
    VALUES
        ('brand_admin', 'entitlement.adjust'),
        ('course_operator', 'entitlement.view'),
        ('finance_support', 'entitlement.adjust')
)
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM new_maps m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (template_id, permission_id) DO NOTHING;

-- 2) BACKFILL 存量 brand：精确按上面 3 条 (template, permission) 给对应 system role 注入。
WITH new_maps(template_code, permission_code) AS (
    VALUES
        ('brand_admin', 'entitlement.adjust'),
        ('course_operator', 'entitlement.view'),
        ('finance_support', 'entitlement.adjust')
)
INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
SELECT br.brand_id, br.id, p.id
FROM new_maps m
JOIN role_templates rt ON rt.code = m.template_code
JOIN brand_roles br ON br.template_id = rt.id
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;
