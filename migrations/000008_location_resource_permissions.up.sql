-- 000008_location_resource_permissions.up.sql
--
-- Batch 12a — Location Resource 资源管理。
--
-- 三表（location_resources / recurring_schedules / recurring_schedule_weekdays）已在
-- 000003 建好，本批不动表结构，仅补 location_resource 细粒度权限 seed（镜像 000007）：
--   1) 4 条 permission（新 domain location_resource）。
--   2) role_template 映射。
--   3) 存量 brand backfill（brand_role_permissions JOIN role_template_permissions）。
--
-- 角色→权限映射：
--   brand_owner / brand_admin / location_manager：全部 4 码（location_manager 管其门店资源）
--   course_operator / instructor / receptionist：location_resource.view（排课需看资源）
-- 注意：location_resource.delete 的 Expand 自动隐含 view+edit；映射仍全列 4 码以保证
--   PermissionSet 表显式一致。

-- 1) 细粒度 permission。
INSERT INTO permissions (code, domain, action, name, description) VALUES
    ('location_resource.view',   'location_resource', 'view',   '查看门店资源', '查看 LocationResource'),
    ('location_resource.create', 'location_resource', 'create', '新增门店资源', '创建 LocationResource'),
    ('location_resource.edit',   'location_resource', 'edit',   '编辑门店资源', '编辑 / 启用停用 LocationResource'),
    ('location_resource.delete', 'location_resource', 'delete', '删除门店资源', '软删 LocationResource')
ON CONFLICT (code) DO NOTHING;

-- 2) role_template 权限映射。
WITH mappings(template_code, permission_code) AS (
    VALUES
        -- brand_owner: 全部 4 码
        ('brand_owner', 'location_resource.view'),
        ('brand_owner', 'location_resource.create'),
        ('brand_owner', 'location_resource.edit'),
        ('brand_owner', 'location_resource.delete'),
        -- brand_admin: 全部 4 码
        ('brand_admin', 'location_resource.view'),
        ('brand_admin', 'location_resource.create'),
        ('brand_admin', 'location_resource.edit'),
        ('brand_admin', 'location_resource.delete'),
        -- location_manager: 全部 4 码（资源属其门店）
        ('location_manager', 'location_resource.view'),
        ('location_manager', 'location_resource.create'),
        ('location_manager', 'location_resource.edit'),
        ('location_manager', 'location_resource.delete'),
        -- course_operator / instructor / receptionist: view（排课需看资源）
        ('course_operator', 'location_resource.view'),
        ('instructor', 'location_resource.view'),
        ('receptionist', 'location_resource.view')
)
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM mappings m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (template_id, permission_id) DO NOTHING;

-- 3) BACKFILL 存量 brand：对每个 brand 的对应 system role 注入新权限。
--    通过 role_template_permissions JOIN，复用 step 2 刚写好的模板映射，保证一致。
INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
SELECT br.brand_id, br.id, rtp.permission_id
FROM brand_roles br
JOIN role_templates rt ON rt.id = br.template_id
JOIN role_template_permissions rtp ON rtp.template_id = rt.id
JOIN permissions p ON p.id = rtp.permission_id
WHERE p.code IN (
    'location_resource.view', 'location_resource.create',
    'location_resource.edit', 'location_resource.delete'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;
