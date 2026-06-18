-- 000011_entitlement_role_mappings.down.sql
--
-- 回滚 000011：删本批补的 3 条 (template, permission) 映射的 brand_role_permissions /
-- role_template_permissions。不删 entitlement.* permission 行本身（000003 建的，非本批）。

WITH old_maps(template_code, permission_code) AS (
    VALUES
        ('brand_admin', 'entitlement.adjust'),
        ('course_operator', 'entitlement.view'),
        ('finance_support', 'entitlement.adjust')
)
DELETE FROM brand_role_permissions brp
USING old_maps m
JOIN role_templates rt ON rt.code = m.template_code
JOIN brand_roles br ON br.template_id = rt.id
JOIN permissions p ON p.code = m.permission_code
WHERE brp.role_id = br.id AND brp.permission_id = p.id;

WITH old_maps(template_code, permission_code) AS (
    VALUES
        ('brand_admin', 'entitlement.adjust'),
        ('course_operator', 'entitlement.view'),
        ('finance_support', 'entitlement.adjust')
)
DELETE FROM role_template_permissions rtp
USING old_maps m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
WHERE rtp.template_id = rt.id AND rtp.permission_id = p.id;
