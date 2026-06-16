-- 000008_location_resource_permissions.down.sql
--
-- 回滚 000008：删 4 条 location_resource permission 的 brand_role_permissions /
-- role_template_permissions 映射，再删 permission 行本身。表结构本批未动，无需回滚。

DELETE FROM brand_role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'location_resource.view', 'location_resource.create',
        'location_resource.edit', 'location_resource.delete'
    )
);

DELETE FROM role_template_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'location_resource.view', 'location_resource.create',
        'location_resource.edit', 'location_resource.delete'
    )
);

DELETE FROM permissions WHERE code IN (
    'location_resource.view', 'location_resource.create',
    'location_resource.edit', 'location_resource.delete'
);
