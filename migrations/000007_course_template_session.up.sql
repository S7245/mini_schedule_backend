-- 000007_course_template_session.up.sql
--
-- Batch 11 — CourseTemplate + CourseCategory + 单场次 ClassSession 排课。
--
-- 1) 放开 courses 健身枚举（difficulty / type DROP NOT NULL），让通用 SaaS 课程
--    模板不再强制健身行业字段（D2）。api-app 只读路径 null→空串，不受影响。
-- 2) courses.status 加 CHECK ('draft','published','archived')（此前无约束），DO $$ 幂等。
-- 3) 9 条细粒度 permission（course.view 已在 000003 存在，复用），ON CONFLICT DO NOTHING。
-- 4) role_template 映射 + 5) 存量 brand backfill（镜像 000005 / 000006 做法）。
--
-- 角色→权限映射（契约 §1.1 step 4）：
--   brand_owner / brand_admin / course_operator：全部 9 码
--   location_manager：course_category.view, course.view, session.view/create/cancel
--   instructor / receptionist：session.view
-- 注意：session.cancel 不在 PermissionSet.Expand 自动隐含表（只有 edit/create→view、
--   delete→view+edit），故任何拿到 session.cancel 的角色必须显式 seed session.view。

-- 1) 放开 courses 健身枚举。
ALTER TABLE courses ALTER COLUMN difficulty DROP NOT NULL;
ALTER TABLE courses ALTER COLUMN type DROP NOT NULL;

-- 2) courses.status CHECK（幂等包裹，镜像 000003 head 的 courses_default_capacity_positive 块）。
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'courses_status_valid'
          AND conrelid = 'courses'::regclass
    ) THEN
        ALTER TABLE courses
            ADD CONSTRAINT courses_status_valid CHECK (status IN ('draft', 'published', 'archived'));
    END IF;
END $$;

-- 3) 细粒度 permission（course.view 已在 000003 存在 → 复用）。
INSERT INTO permissions (code, domain, action, name, description) VALUES
    ('course_category.view',   'course_category', 'view',   '查看课程分类', '查看 CourseCategory'),
    ('course_category.create', 'course_category', 'create', '新增课程分类', '创建 CourseCategory'),
    ('course_category.edit',   'course_category', 'edit',   '编辑课程分类', '编辑 / 启用停用 CourseCategory'),
    ('course.create',          'course',          'create', '新增课程模板', '创建 CourseTemplate'),
    ('course.edit',            'course',          'edit',   '编辑课程模板', '编辑 / 发布 / 归档 CourseTemplate'),
    ('course.delete',          'course',          'delete', '删除课程模板', '软删 CourseTemplate'),
    ('session.view',           'session',         'view',   '查看课程场次', '查看 ClassSession'),
    ('session.create',         'session',         'create', '排课/新增场次', '创建 ClassSession'),
    ('session.cancel',         'session',         'cancel', '取消场次',      '取消 ClassSession')
ON CONFLICT (code) DO NOTHING;

-- 4) role_template 权限映射。
WITH mappings(template_code, permission_code) AS (
    VALUES
        -- brand_owner: 全部 9 码
        ('brand_owner', 'course_category.view'),
        ('brand_owner', 'course_category.create'),
        ('brand_owner', 'course_category.edit'),
        ('brand_owner', 'course.view'),
        ('brand_owner', 'course.create'),
        ('brand_owner', 'course.edit'),
        ('brand_owner', 'course.delete'),
        ('brand_owner', 'session.view'),
        ('brand_owner', 'session.create'),
        ('brand_owner', 'session.cancel'),
        -- brand_admin: 全部 9 码
        ('brand_admin', 'course_category.view'),
        ('brand_admin', 'course_category.create'),
        ('brand_admin', 'course_category.edit'),
        ('brand_admin', 'course.view'),
        ('brand_admin', 'course.create'),
        ('brand_admin', 'course.edit'),
        ('brand_admin', 'course.delete'),
        ('brand_admin', 'session.view'),
        ('brand_admin', 'session.create'),
        ('brand_admin', 'session.cancel'),
        -- course_operator: 全部 9 码
        ('course_operator', 'course_category.view'),
        ('course_operator', 'course_category.create'),
        ('course_operator', 'course_category.edit'),
        ('course_operator', 'course.view'),
        ('course_operator', 'course.create'),
        ('course_operator', 'course.edit'),
        ('course_operator', 'course.delete'),
        ('course_operator', 'session.view'),
        ('course_operator', 'session.create'),
        ('course_operator', 'session.cancel'),
        -- location_manager: course_category.view + course.view + session.view/create/cancel
        ('location_manager', 'course_category.view'),
        ('location_manager', 'course.view'),
        ('location_manager', 'session.view'),
        ('location_manager', 'session.create'),
        ('location_manager', 'session.cancel'),
        -- instructor / receptionist: session.view
        ('instructor', 'session.view'),
        ('receptionist', 'session.view')
)
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM mappings m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (template_id, permission_id) DO NOTHING;

-- 5) BACKFILL 存量 brand：对每个 brand 的对应 system role 注入新权限。
--    通过 role_template_permissions JOIN，复用 step 4 刚写好的模板映射，保证一致。
INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
SELECT br.brand_id, br.id, rtp.permission_id
FROM brand_roles br
JOIN role_templates rt ON rt.id = br.template_id
JOIN role_template_permissions rtp ON rtp.template_id = rt.id
JOIN permissions p ON p.id = rtp.permission_id
WHERE p.code IN (
    'course_category.view', 'course_category.create', 'course_category.edit',
    'course.create', 'course.edit', 'course.delete',
    'session.view', 'session.create', 'session.cancel'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;
