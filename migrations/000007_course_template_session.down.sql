-- 000007_course_template_session.down.sql
--
-- 回滚 000007：
--   1) 删 9 条新 permission 的 brand_role_permissions / role_template_permissions 映射，
--      再删 permission 行本身（course.view 是 000003 的，不在此列，保留）。
--   2) 删 courses_status_valid CHECK。
--
-- 注意：difficulty / type 的 DROP NOT NULL 不安全可逆——回滚后表里可能已存在 NULL
--   值（本批新建的课程模板不写这两列）。强行 SET NOT NULL 会失败或丢数据，故 down 不
--   重加 NOT NULL，保持最小且文档化。

-- 1) 删新权限的所有引用 + 权限本身（course.view 排除在外）。
DELETE FROM brand_role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'course_category.view', 'course_category.create', 'course_category.edit',
        'course.create', 'course.edit', 'course.delete',
        'session.view', 'session.create', 'session.cancel'
    )
);

DELETE FROM role_template_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'course_category.view', 'course_category.create', 'course_category.edit',
        'course.create', 'course.edit', 'course.delete',
        'session.view', 'session.create', 'session.cancel'
    )
);

DELETE FROM permissions WHERE code IN (
    'course_category.view', 'course_category.create', 'course_category.edit',
    'course.create', 'course.edit', 'course.delete',
    'session.view', 'session.create', 'session.cancel'
);

-- 2) 删 status CHECK。
ALTER TABLE courses DROP CONSTRAINT IF EXISTS courses_status_valid;

-- difficulty / type 的 NOT NULL 不重加（见文件头说明）。
