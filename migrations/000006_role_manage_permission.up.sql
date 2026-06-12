-- 000006_role_manage_permission.up.sql
--
-- Batch 7 — 品牌自定义角色 CRUD 的写接口权限门。
--
-- 新增一条系统级细粒度 permission `role.manage`（domain=role）作为
-- POST/PUT/PATCH/DELETE /roles + GET /permissions 的权限门，并：
--   1) 映射到 role_templates code IN ('brand_owner','brand_admin')；
--   2) BACKFILL 存量 brand 的 brand_roles（同样 code 范围）→ brand_role_permissions，
--      否则 000006 之前已 seed 的 brand 里 brand_admin 看不到角色管理入口
--      （owner 走 fast-path 本不需要，但 brand_admin 必须 backfill）。
--
-- 所有写都 ON CONFLICT DO NOTHING，保证幂等 + 可重复执行。

-- 1) permission 行。
INSERT INTO permissions (code, domain, action, name, description) VALUES
    ('role.manage', 'role', 'manage', '角色管理', '创建/编辑/删除自定义角色')
ON CONFLICT (code) DO NOTHING;

-- 2) role_templates 映射（brand_owner / brand_admin）。
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM role_templates rt
JOIN permissions p ON p.code = 'role.manage'
WHERE rt.code IN ('brand_owner', 'brand_admin')
ON CONFLICT (template_id, permission_id) DO NOTHING;

-- 3) BACKFILL 存量 brand：对所有 brand_roles code IN ('brand_owner','brand_admin')
--    注入 role.manage。
INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
SELECT br.brand_id, br.id, p.id
FROM brand_roles br
JOIN permissions p ON p.code = 'role.manage'
WHERE br.code IN ('brand_owner', 'brand_admin')
ON CONFLICT (role_id, permission_id) DO NOTHING;
