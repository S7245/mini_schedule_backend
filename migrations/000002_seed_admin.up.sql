-- 000002_seed_admin.up.sql
-- 初始化超级管理员账号（密码：Admin@123456）
-- 使用 pgcrypto 生成 bcrypt hash，与 Go 的 bcrypt 完全兼容

CREATE EXTENSION IF NOT EXISTS pgcrypto;

INSERT INTO admin_users (username, password_hash, role, status, created_at, updated_at)
VALUES (
    'admin',
    crypt('Admin@123456', gen_salt('bf', 10)),
    'super_admin',
    'active',
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;
