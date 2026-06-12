-- 000006_role_manage_permission.down.sql
--
-- 回滚 000006：移除 role.manage 的 brand_role_permissions / role_template_permissions
-- 映射，再删 permission 行本身。

DELETE FROM brand_role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'role.manage');

DELETE FROM role_template_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'role.manage');

DELETE FROM permissions WHERE code = 'role.manage';
