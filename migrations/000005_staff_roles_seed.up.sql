-- 000005_staff_roles_seed.up.sql
--
-- Batch 5 — Staff/Role seed expansion.
--
-- 注意：
-- 1) brand_users.is_owner 列已在 000003 加过，但本批继续 ADD COLUMN IF NOT EXISTS 兜底
--    （新环境若跳过 000003 中段的 ALTER 仍能拿到列）。
-- 2) 000003 已 seed 一套粗粒度 permissions / role_templates（brand.view、staff.manage、
--    instructor、receptionist 等）。Batch 5 补一份"细粒度"权限码（按 action 拆 view / create
--    / edit / delete / toggle_status / assign_role / assign_location 等），用于 Staff 模块
--    的菜单与按钮控制。两套并存：粗粒度归 Batch 3 占位，细粒度归 Batch 5 落地；通过 code
--    唯一确保不会重复插入。
-- 3) 8 个标准 role_templates 已在 000003 seed；其中 4 个的 code 与 Batch 5 蓝图不同：
--    finance_support (=Batch5 finance_aftercare)、instructor (=Batch5 location_instructor)、
--    receptionist (=Batch5 location_reception)。本批保留 000003 的命名，只把 Batch 5 细粒
--    度权限映射挂到这些既有 template 上（避免双轨）。
-- 4) 040003 已自动 backfill 现有 brand 的 brand_roles + brand_role_permissions。Batch 5
--    应用层 role_allocator 负责把"细粒度新增的 brand_role_permissions"补给现有 brand。
--    （SQL 写起来需要重新 CROSS JOIN brands × 新 permission，复杂度不值得；用幂等服务跑。）

-- 1) brand_users.is_owner —— 000003 已加；幂等再确认。
ALTER TABLE brand_users ADD COLUMN IF NOT EXISTS is_owner BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_brand_users_brand_owner ON brand_users(brand_id) WHERE is_owner = TRUE;

-- 2) Batch 5 细粒度 permissions（11 条；与 000003 粗粒度并存）。
INSERT INTO permissions (code, domain, action, name, description) VALUES
    ('brand.profile.view',     'brand',      'view',            '查看品牌资料',    '查看 Brand 基本信息'),
    ('brand.profile.edit',     'brand',      'edit',            '编辑品牌资料',    '修改 Brand 资料字段'),
    ('location.create',        'location',   'create',          '新增门店',        '创建 Location'),
    ('location.edit',          'location',   'edit',            '编辑门店',        '修改 Location 基本信息'),
    ('location.delete',        'location',   'delete',          '删除门店',        '软删 Location'),
    ('location.toggle_status', 'location',   'toggle_status',   '启用/停用门店',   '切换 Location 启用状态'),
    ('staff.create',           'staff',      'create',          '新增员工',        '创建 Staff'),
    ('staff.edit',             'staff',      'edit',            '编辑员工',        '修改 Staff 基本信息'),
    ('staff.delete',           'staff',      'delete',          '删除员工',        '软删 Staff（owner 不可删）'),
    ('staff.assign_role',      'staff',      'assign_role',     '分配角色',        '给 Staff 分配 / 移除角色'),
    ('staff.assign_location',  'staff',      'assign_location', '分配门店任职',    '给 Staff 分配 / 移除 Location'),
    ('instructor.view',        'instructor', 'view',            '查看教练档案',    '查看 InstructorProfile'),
    ('instructor.edit',        'instructor', 'edit',            '维护教练档案',    '维护 InstructorProfile（创建/编辑/启用停用）')
ON CONFLICT (code) DO NOTHING;

-- 3) Batch 5 → role_template 权限映射（仅给 active templates 加细粒度 permissions）。
--    使用 WITH mappings 的 VALUES 表达式 + JOIN，与 000003 同模式。
WITH mappings(template_code, permission_code) AS (
    VALUES
        -- brand_owner: 全套细粒度权限
        ('brand_owner', 'brand.profile.view'),
        ('brand_owner', 'brand.profile.edit'),
        ('brand_owner', 'location.create'),
        ('brand_owner', 'location.edit'),
        ('brand_owner', 'location.delete'),
        ('brand_owner', 'location.toggle_status'),
        ('brand_owner', 'staff.create'),
        ('brand_owner', 'staff.edit'),
        ('brand_owner', 'staff.delete'),
        ('brand_owner', 'staff.assign_role'),
        ('brand_owner', 'staff.assign_location'),
        ('brand_owner', 'instructor.view'),
        ('brand_owner', 'instructor.edit'),
        -- brand_admin: 除 staff.delete 外的全部
        ('brand_admin', 'brand.profile.view'),
        ('brand_admin', 'brand.profile.edit'),
        ('brand_admin', 'location.create'),
        ('brand_admin', 'location.edit'),
        ('brand_admin', 'location.toggle_status'),
        ('brand_admin', 'staff.create'),
        ('brand_admin', 'staff.edit'),
        ('brand_admin', 'staff.assign_role'),
        ('brand_admin', 'staff.assign_location'),
        ('brand_admin', 'instructor.view'),
        ('brand_admin', 'instructor.edit'),
        -- location_manager: 本店 location + staff + instructor
        ('location_manager', 'location.edit'),
        ('location_manager', 'location.toggle_status'),
        ('location_manager', 'staff.assign_location'),
        ('location_manager', 'instructor.view'),
        ('location_manager', 'instructor.edit'),
        -- instructor (Batch5 location_instructor): 只看自己 + 维护自己档案
        ('instructor', 'instructor.view'),
        ('instructor', 'instructor.edit')
)
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM mappings m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (template_id, permission_id) DO NOTHING;
