-- 000005_staff_roles_seed.down.sql
--
-- 回滚：只回滚 Batch 5 引入的细粒度 permissions 和它们的 role_template_permissions 行；
-- 不动 000003 已经存在的 brand_users.is_owner 列（避免破坏更早依赖）。

DELETE FROM role_template_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'brand.profile.view','brand.profile.edit',
        'location.create','location.edit','location.delete','location.toggle_status',
        'staff.create','staff.edit','staff.delete','staff.assign_role','staff.assign_location',
        'instructor.view','instructor.edit'
    )
);

DELETE FROM brand_role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'brand.profile.view','brand.profile.edit',
        'location.create','location.edit','location.delete','location.toggle_status',
        'staff.create','staff.edit','staff.delete','staff.assign_role','staff.assign_location',
        'instructor.view','instructor.edit'
    )
);

DELETE FROM permissions WHERE code IN (
    'brand.profile.view','brand.profile.edit',
    'location.create','location.edit','location.delete','location.toggle_status',
    'staff.create','staff.edit','staff.delete','staff.assign_role','staff.assign_location',
    'instructor.view','instructor.edit'
);
