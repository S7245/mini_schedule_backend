-- 000001_init_schema.up.sql
-- 初始化全部表结构

-- 品牌表
CREATE TABLE IF NOT EXISTS brands (
    id           BIGSERIAL PRIMARY KEY,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ,
    name         VARCHAR(100) NOT NULL,
    logo_url     VARCHAR(500),
    contact_name  VARCHAR(50)  NOT NULL,
    contact_phone VARCHAR(20)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending'
);
CREATE INDEX  IF NOT EXISTS idx_brands_name         ON brands(name);
CREATE INDEX  IF NOT EXISTS idx_brands_status       ON brands(status);
CREATE INDEX  IF NOT EXISTS idx_brands_deleted_at   ON brands(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brands_contact_phone ON brands(contact_phone);

-- 品牌管理员表
CREATE TABLE IF NOT EXISTS brand_users (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ,
    brand_id      BIGINT       NOT NULL,
    phone         VARCHAR(20)  NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name          VARCHAR(50)  NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'active'
);
CREATE INDEX  IF NOT EXISTS idx_brand_users_brand_id   ON brand_users(brand_id);
CREATE INDEX  IF NOT EXISTS idx_brand_users_deleted_at ON brand_users(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_users_phone ON brand_users(phone);

-- C 端用户表
CREATE TABLE IF NOT EXISTS app_users (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    brand_id    BIGINT      NOT NULL,
    open_id     VARCHAR(100) NOT NULL,
    phone       VARCHAR(20),
    nickname    VARCHAR(50),
    avatar_url  VARCHAR(500),
    vip_level   VARCHAR(20) NOT NULL DEFAULT 'free',
    status      VARCHAR(20) NOT NULL DEFAULT 'active'
);
CREATE INDEX  IF NOT EXISTS idx_app_users_brand_id   ON app_users(brand_id);
CREATE INDEX  IF NOT EXISTS idx_app_users_phone      ON app_users(phone);
CREATE INDEX  IF NOT EXISTS idx_app_users_deleted_at ON app_users(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_openid   ON app_users(brand_id, open_id);

-- 平台管理员表
CREATE TABLE IF NOT EXISTS admin_users (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ,
    username      VARCHAR(50)  NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(20)  NOT NULL DEFAULT 'operator',
    status        VARCHAR(20)  NOT NULL DEFAULT 'active'
);
CREATE INDEX  IF NOT EXISTS idx_admin_users_deleted_at ON admin_users(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_username ON admin_users(username);

-- 课程表
CREATE TABLE IF NOT EXISTS courses (
    id           BIGSERIAL PRIMARY KEY,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ,
    brand_id     BIGINT       NOT NULL,
    title        VARCHAR(200) NOT NULL,
    description  VARCHAR(2000),
    cover_url    VARCHAR(500),
    difficulty   VARCHAR(20)  NOT NULL,
    duration_min INT          NOT NULL,
    type         VARCHAR(20)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'draft'
);
CREATE INDEX IF NOT EXISTS idx_courses_brand_id   ON courses(brand_id);
CREATE INDEX IF NOT EXISTS idx_courses_title      ON courses(title);
CREATE INDEX IF NOT EXISTS idx_courses_type       ON courses(type);
CREATE INDEX IF NOT EXISTS idx_courses_status     ON courses(status);
CREATE INDEX IF NOT EXISTS idx_courses_deleted_at ON courses(deleted_at);

-- 训练记录表
CREATE TABLE IF NOT EXISTS training_records (
    id           BIGSERIAL PRIMARY KEY,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ,
    user_id      BIGINT      NOT NULL,
    brand_id     BIGINT      NOT NULL,
    course_id    BIGINT      NOT NULL,
    duration_min INT         NOT NULL,
    calories     FLOAT       NOT NULL DEFAULT 0,
    notes        VARCHAR(500),
    completed_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_training_records_user_id      ON training_records(user_id);
CREATE INDEX IF NOT EXISTS idx_training_records_brand_id     ON training_records(brand_id);
CREATE INDEX IF NOT EXISTS idx_training_records_course_id    ON training_records(course_id);
CREATE INDEX IF NOT EXISTS idx_training_records_completed_at ON training_records(completed_at);
CREATE INDEX IF NOT EXISTS idx_training_records_deleted_at   ON training_records(deleted_at);
