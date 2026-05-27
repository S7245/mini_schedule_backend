-- 000003_course_booking_schema.up.sql
-- Course booking SaaS schema expansion.

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Existing core tables are kept as the product anchors:
-- brands = Brand, brand_users = Staff, courses = CourseTemplate.
ALTER TABLE brands
    ADD COLUMN IF NOT EXISTS brand_code VARCHAR(50),
    ADD COLUMN IF NOT EXISTS contact_email VARCHAR(100),
    ADD COLUMN IF NOT EXISTS industry_type VARCHAR(50),
    ADD COLUMN IF NOT EXISTS onboarding_status VARCHAR(30) NOT NULL DEFAULT 'not_started',
    ADD COLUMN IF NOT EXISTS onboarding_completed_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_brands_brand_code
    ON brands(brand_code)
    WHERE brand_code IS NOT NULL;

ALTER TABLE brand_users
    ADD COLUMN IF NOT EXISTS email VARCHAR(100),
    ADD COLUMN IF NOT EXISTS avatar_url VARCHAR(500),
    ADD COLUMN IF NOT EXISTS is_owner BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;

ALTER TABLE courses
    ADD COLUMN IF NOT EXISTS default_capacity INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS level_label VARCHAR(50),
    ADD COLUMN IF NOT EXISTS show_in_mini_program BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'courses_default_capacity_positive'
          AND conrelid = 'courses'::regclass
    ) THEN
        ALTER TABLE courses
            ADD CONSTRAINT courses_default_capacity_positive CHECK (default_capacity > 0);
    END IF;
END $$;

-- Platform commercialization.
CREATE TABLE IF NOT EXISTS saas_plans (
    id                 BIGSERIAL PRIMARY KEY,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name               VARCHAR(100) NOT NULL,
    description        VARCHAR(1000),
    monthly_price      NUMERIC(12, 2) NOT NULL DEFAULT 0,
    yearly_price       NUMERIC(12, 2) NOT NULL DEFAULT 0,
    yearly_discount_pct NUMERIC(5, 2),
    currency           CHAR(3) NOT NULL DEFAULT 'CNY',
    max_locations      INT NOT NULL,
    max_staff_seats    INT NOT NULL,
    max_learners       INT NOT NULL,
    status             VARCHAR(20) NOT NULL DEFAULT 'active',
    sort_order         INT NOT NULL DEFAULT 0,
    CONSTRAINT saas_plans_price_non_negative CHECK (
        monthly_price >= 0
        AND yearly_price >= 0
        AND (yearly_discount_pct IS NULL OR (yearly_discount_pct >= 0 AND yearly_discount_pct <= 100))
    ),
    CONSTRAINT saas_plans_limits_positive CHECK (
        max_locations > 0
        AND max_staff_seats > 0
        AND max_learners > 0
    ),
    CONSTRAINT saas_plans_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_saas_plans_status_sort ON saas_plans(status, sort_order);

CREATE TABLE IF NOT EXISTS saas_plan_features (
    id           BIGSERIAL PRIMARY KEY,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    plan_id      BIGINT NOT NULL REFERENCES saas_plans(id) ON DELETE CASCADE,
    feature_code VARCHAR(80) NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT saas_plan_features_code_not_blank CHECK (length(trim(feature_code)) > 0)
);
CREATE INDEX IF NOT EXISTS idx_saas_plan_features_plan_id ON saas_plan_features(plan_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_saas_plan_features_plan_code ON saas_plan_features(plan_id, feature_code);

CREATE TABLE IF NOT EXISTS saas_plan_orders (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id              BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_user_id         BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    plan_id               BIGINT NOT NULL REFERENCES saas_plans(id) ON DELETE RESTRICT,
    source                VARCHAR(40) NOT NULL DEFAULT 'public_signup_first_purchase',
    billing_cycle         VARCHAR(20) NOT NULL,
    amount                NUMERIC(12, 2) NOT NULL,
    currency              CHAR(3) NOT NULL DEFAULT 'CNY',
    payment_channel       VARCHAR(20) NOT NULL,
    status                VARCHAR(30) NOT NULL DEFAULT 'pending_payment',
    out_trade_no          VARCHAR(100) NOT NULL,
    third_party_trade_no  VARCHAR(100),
    wechat_code_url       VARCHAR(500),
    wechat_prepay_id      VARCHAR(100),
    payment_request_payload JSONB,
    payment_response_payload JSONB,
    payment_expires_at    TIMESTAMPTZ,
    paid_at               TIMESTAMPTZ,
    closed_at             TIMESTAMPTZ,
    failure_reason        VARCHAR(500),
    CONSTRAINT saas_plan_orders_source_valid CHECK (
        source IN (
            'public_signup_first_purchase',
            'admin_manual_compensation',
            'brand_self_service_renewal',
            'brand_self_service_upgrade',
            'brand_self_service_downgrade'
        )
    ),
    CONSTRAINT saas_plan_orders_billing_cycle_valid CHECK (billing_cycle IN ('monthly', 'yearly')),
    CONSTRAINT saas_plan_orders_amount_non_negative CHECK (amount >= 0),
    CONSTRAINT saas_plan_orders_payment_channel_valid CHECK (payment_channel IN ('wechat', 'alipay')),
    CONSTRAINT saas_plan_orders_status_valid CHECK (
        status IN ('pending_payment', 'paid', 'closed', 'failed', 'refunding', 'refunded', 'exception')
    )
);
CREATE INDEX IF NOT EXISTS idx_saas_plan_orders_brand_id ON saas_plan_orders(brand_id);
CREATE INDEX IF NOT EXISTS idx_saas_plan_orders_brand_user_id ON saas_plan_orders(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_saas_plan_orders_plan_id ON saas_plan_orders(plan_id);
CREATE INDEX IF NOT EXISTS idx_saas_plan_orders_source_status ON saas_plan_orders(source, status);
CREATE INDEX IF NOT EXISTS idx_saas_plan_orders_status_created_at ON saas_plan_orders(status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_saas_plan_orders_out_trade_no ON saas_plan_orders(out_trade_no);
CREATE UNIQUE INDEX IF NOT EXISTS idx_saas_plan_orders_third_party_trade_no
    ON saas_plan_orders(third_party_trade_no)
    WHERE third_party_trade_no IS NOT NULL;

CREATE TABLE IF NOT EXISTS brand_subscriptions (
    id                 BIGSERIAL PRIMARY KEY,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id           BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    plan_id            BIGINT NOT NULL REFERENCES saas_plans(id) ON DELETE RESTRICT,
    order_id           BIGINT REFERENCES saas_plan_orders(id) ON DELETE SET NULL,
    billing_cycle      VARCHAR(20) NOT NULL,
    status             VARCHAR(30) NOT NULL DEFAULT 'active',
    starts_at          TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    grace_ends_at      TIMESTAMPTZ,
    max_locations      INT NOT NULL,
    max_staff_seats    INT NOT NULL,
    max_learners       INT NOT NULL,
    frozen_reason      VARCHAR(500),
    CONSTRAINT brand_subscriptions_billing_cycle_valid CHECK (billing_cycle IN ('monthly', 'yearly')),
    CONSTRAINT brand_subscriptions_status_valid CHECK (
        status IN ('active', 'grace_period', 'restricted', 'frozen', 'expired', 'cancelled')
    ),
    CONSTRAINT brand_subscriptions_period_valid CHECK (expires_at > starts_at),
    CONSTRAINT brand_subscriptions_limits_positive CHECK (
        max_locations > 0
        AND max_staff_seats > 0
        AND max_learners > 0
    )
);
CREATE INDEX IF NOT EXISTS idx_brand_subscriptions_brand_id ON brand_subscriptions(brand_id);
CREATE INDEX IF NOT EXISTS idx_brand_subscriptions_plan_id ON brand_subscriptions(plan_id);
CREATE INDEX IF NOT EXISTS idx_brand_subscriptions_order_id ON brand_subscriptions(order_id);
CREATE INDEX IF NOT EXISTS idx_brand_subscriptions_status_expires_at ON brand_subscriptions(status, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_subscriptions_one_current
    ON brand_subscriptions(brand_id)
    WHERE status IN ('active', 'grace_period', 'restricted', 'frozen');

CREATE TABLE IF NOT EXISTS brand_subscription_features (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    subscription_id BIGINT NOT NULL REFERENCES brand_subscriptions(id) ON DELETE CASCADE,
    feature_code    VARCHAR(80) NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT brand_subscription_features_code_not_blank CHECK (length(trim(feature_code)) > 0)
);
CREATE INDEX IF NOT EXISTS idx_brand_subscription_features_subscription_id
    ON brand_subscription_features(subscription_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_subscription_features_subscription_code
    ON brand_subscription_features(subscription_id, feature_code);

CREATE TABLE IF NOT EXISTS payment_transactions (
    id                   BIGSERIAL PRIMARY KEY,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id             BIGINT REFERENCES brands(id) ON DELETE SET NULL,
    order_id             BIGINT REFERENCES saas_plan_orders(id) ON DELETE SET NULL,
    payment_channel      VARCHAR(20) NOT NULL,
    transaction_type     VARCHAR(20) NOT NULL DEFAULT 'payment',
    status               VARCHAR(30) NOT NULL DEFAULT 'pending',
    amount               NUMERIC(12, 2) NOT NULL,
    currency             CHAR(3) NOT NULL DEFAULT 'CNY',
    out_trade_no         VARCHAR(100) NOT NULL,
    third_party_trade_no VARCHAR(100),
    provider_request_id  VARCHAR(100),
    request_payload      JSONB,
    response_payload     JSONB,
    callback_payload     JSONB,
    callback_received_at TIMESTAMPTZ,
    paid_at              TIMESTAMPTZ,
    failure_reason       VARCHAR(500),
    CONSTRAINT payment_transactions_channel_valid CHECK (payment_channel IN ('wechat', 'alipay')),
    CONSTRAINT payment_transactions_type_valid CHECK (transaction_type IN ('payment', 'refund')),
    CONSTRAINT payment_transactions_status_valid CHECK (
        status IN ('pending', 'succeeded', 'failed', 'closed', 'refunding', 'refunded', 'exception')
    ),
    CONSTRAINT payment_transactions_amount_non_negative CHECK (amount >= 0)
);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_brand_id ON payment_transactions(brand_id);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_order_id ON payment_transactions(order_id);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_status_created_at ON payment_transactions(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_provider_request_id
    ON payment_transactions(provider_request_id)
    WHERE provider_request_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_transactions_channel_trade_type
    ON payment_transactions(payment_channel, out_trade_no, transaction_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_transactions_third_party_trade_no
    ON payment_transactions(third_party_trade_no)
    WHERE third_party_trade_no IS NOT NULL;

CREATE TABLE IF NOT EXISTS payment_callback_logs (
    id                   BIGSERIAL PRIMARY KEY,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id             BIGINT REFERENCES brands(id) ON DELETE SET NULL,
    order_id             BIGINT REFERENCES saas_plan_orders(id) ON DELETE SET NULL,
    transaction_id       BIGINT REFERENCES payment_transactions(id) ON DELETE SET NULL,
    payment_channel      VARCHAR(20) NOT NULL,
    out_trade_no         VARCHAR(100),
    third_party_trade_no VARCHAR(100),
    callback_request_id  VARCHAR(100),
    status               VARCHAR(30) NOT NULL DEFAULT 'received',
    headers              JSONB NOT NULL DEFAULT '{}'::JSONB,
    raw_body             TEXT,
    payload              JSONB NOT NULL DEFAULT '{}'::JSONB,
    processed_at         TIMESTAMPTZ,
    error_message        VARCHAR(1000),
    CONSTRAINT payment_callback_logs_channel_valid CHECK (payment_channel IN ('wechat', 'alipay')),
    CONSTRAINT payment_callback_logs_status_valid CHECK (status IN ('received', 'processed', 'failed', 'ignored'))
);
CREATE INDEX IF NOT EXISTS idx_payment_callback_logs_brand_id ON payment_callback_logs(brand_id);
CREATE INDEX IF NOT EXISTS idx_payment_callback_logs_order_id ON payment_callback_logs(order_id);
CREATE INDEX IF NOT EXISTS idx_payment_callback_logs_transaction_id ON payment_callback_logs(transaction_id);
CREATE INDEX IF NOT EXISTS idx_payment_callback_logs_status_created_at ON payment_callback_logs(status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_callback_logs_request_id
    ON payment_callback_logs(payment_channel, callback_request_id)
    WHERE callback_request_id IS NOT NULL;

-- Brand onboarding.
CREATE TABLE IF NOT EXISTS brand_onboarding_steps (
    id           BIGSERIAL PRIMARY KEY,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id     BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    step_key     VARCHAR(80) NOT NULL,
    status       VARCHAR(30) NOT NULL DEFAULT 'not_started',
    completed_at TIMESTAMPTZ,
    skipped_at   TIMESTAMPTZ,
    metadata     JSONB NOT NULL DEFAULT '{}'::JSONB,
    CONSTRAINT brand_onboarding_steps_status_valid CHECK (
        status IN ('not_started', 'in_progress', 'completed', 'skipped')
    )
);
CREATE INDEX IF NOT EXISTS idx_brand_onboarding_steps_brand_id ON brand_onboarding_steps(brand_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_onboarding_steps_brand_step ON brand_onboarding_steps(brand_id, step_key);

-- Locations and resources.
CREATE TABLE IF NOT EXISTS locations (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    brand_id   BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    address    VARCHAR(500),
    phone      VARCHAR(20),
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    remark     VARCHAR(1000),
    CONSTRAINT locations_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_locations_brand_id ON locations(brand_id);
CREATE INDEX IF NOT EXISTS idx_locations_brand_status ON locations(brand_id, status);
CREATE INDEX IF NOT EXISTS idx_locations_deleted_at ON locations(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_locations_brand_name_active
    ON locations(brand_id, name)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS location_resources (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    brand_id    BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    location_id BIGINT NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    type        VARCHAR(30) NOT NULL,
    capacity    INT NOT NULL DEFAULT 1,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    remark      VARCHAR(1000),
    CONSTRAINT location_resources_type_valid CHECK (type IN ('classroom', 'venue', 'online', 'equipment', 'other')),
    CONSTRAINT location_resources_status_valid CHECK (status IN ('active', 'inactive')),
    CONSTRAINT location_resources_capacity_positive CHECK (capacity > 0)
);
CREATE INDEX IF NOT EXISTS idx_location_resources_brand_id ON location_resources(brand_id);
CREATE INDEX IF NOT EXISTS idx_location_resources_location_id ON location_resources(location_id);
CREATE INDEX IF NOT EXISTS idx_location_resources_location_status ON location_resources(location_id, status);
CREATE INDEX IF NOT EXISTS idx_location_resources_deleted_at ON location_resources(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_location_resources_location_name_active
    ON location_resources(location_id, name)
    WHERE deleted_at IS NULL;

-- Role templates, permissions, and staff assignments.
CREATE TABLE IF NOT EXISTS permissions (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    code        VARCHAR(80) NOT NULL,
    domain      VARCHAR(50) NOT NULL,
    action      VARCHAR(50) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT permissions_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_permissions_code ON permissions(code);
CREATE INDEX IF NOT EXISTS idx_permissions_domain_action ON permissions(domain, action);

CREATE TABLE IF NOT EXISTS role_templates (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    code        VARCHAR(80) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    scope_type  VARCHAR(20) NOT NULL,
    description VARCHAR(500),
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT role_templates_scope_valid CHECK (scope_type IN ('brand', 'location')),
    CONSTRAINT role_templates_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_templates_code ON role_templates(code);
CREATE INDEX IF NOT EXISTS idx_role_templates_scope_status ON role_templates(scope_type, status);

CREATE TABLE IF NOT EXISTS role_template_permissions (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    template_id BIGINT NOT NULL REFERENCES role_templates(id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_role_template_permissions_template_id ON role_template_permissions(template_id);
CREATE INDEX IF NOT EXISTS idx_role_template_permissions_permission_id ON role_template_permissions(permission_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_template_permissions_unique
    ON role_template_permissions(template_id, permission_id);

CREATE TABLE IF NOT EXISTS brand_roles (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id    BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    template_id BIGINT REFERENCES role_templates(id) ON DELETE SET NULL,
    code        VARCHAR(80) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    scope_type  VARCHAR(20) NOT NULL,
    is_system   BOOLEAN NOT NULL DEFAULT TRUE,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    description VARCHAR(500),
    CONSTRAINT brand_roles_scope_valid CHECK (scope_type IN ('brand', 'location')),
    CONSTRAINT brand_roles_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_brand_roles_brand_id ON brand_roles(brand_id);
CREATE INDEX IF NOT EXISTS idx_brand_roles_template_id ON brand_roles(template_id);
CREATE INDEX IF NOT EXISTS idx_brand_roles_brand_scope_status ON brand_roles(brand_id, scope_type, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_roles_brand_code ON brand_roles(brand_id, code);

CREATE TABLE IF NOT EXISTS brand_role_permissions (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id      BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    role_id       BIGINT NOT NULL REFERENCES brand_roles(id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_brand_role_permissions_brand_id ON brand_role_permissions(brand_id);
CREATE INDEX IF NOT EXISTS idx_brand_role_permissions_role_id ON brand_role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_brand_role_permissions_permission_id ON brand_role_permissions(permission_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_role_permissions_unique
    ON brand_role_permissions(role_id, permission_id);

CREATE TABLE IF NOT EXISTS staff_location_assignments (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id      BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_user_id BIGINT NOT NULL REFERENCES brand_users(id) ON DELETE CASCADE,
    location_id   BIGINT NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    assignment_type VARCHAR(30) NOT NULL DEFAULT 'member',
    is_primary    BOOLEAN NOT NULL DEFAULT FALSE,
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT staff_location_assignments_type_valid CHECK (
        assignment_type IN ('member', 'manager', 'instructor', 'assistant')
    ),
    CONSTRAINT staff_location_assignments_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_staff_location_assignments_brand_id ON staff_location_assignments(brand_id);
CREATE INDEX IF NOT EXISTS idx_staff_location_assignments_brand_user_id ON staff_location_assignments(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_staff_location_assignments_location_id ON staff_location_assignments(location_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_location_assignments_active_unique
    ON staff_location_assignments(brand_user_id, location_id, assignment_type)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS brand_user_role_assignments (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id      BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_user_id BIGINT NOT NULL REFERENCES brand_users(id) ON DELETE CASCADE,
    role_id       BIGINT NOT NULL REFERENCES brand_roles(id) ON DELETE CASCADE,
    location_id   BIGINT REFERENCES locations(id) ON DELETE CASCADE,
    data_scope    VARCHAR(30) NOT NULL DEFAULT 'role_default',
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT brand_user_role_assignments_scope_valid CHECK (
        data_scope IN ('role_default', 'all_brand', 'assigned_locations', 'own_sessions', 'own_records')
    ),
    CONSTRAINT brand_user_role_assignments_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_brand_user_role_assignments_brand_id ON brand_user_role_assignments(brand_id);
CREATE INDEX IF NOT EXISTS idx_brand_user_role_assignments_brand_user_id ON brand_user_role_assignments(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_brand_user_role_assignments_role_id ON brand_user_role_assignments(role_id);
CREATE INDEX IF NOT EXISTS idx_brand_user_role_assignments_location_id ON brand_user_role_assignments(location_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_user_role_assignments_active_brand_role
    ON brand_user_role_assignments(brand_user_id, role_id)
    WHERE status = 'active' AND location_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_user_role_assignments_active_location_role
    ON brand_user_role_assignments(brand_user_id, role_id, location_id)
    WHERE status = 'active' AND location_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS instructor_profiles (
    id                     BIGSERIAL PRIMARY KEY,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id               BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_user_id          BIGINT NOT NULL REFERENCES brand_users(id) ON DELETE CASCADE,
    display_name           VARCHAR(100) NOT NULL,
    avatar_url             VARCHAR(500),
    bio                    VARCHAR(2000),
    specialties            VARCHAR(1000),
    certificates           VARCHAR(1000),
    is_visible_to_learners BOOLEAN NOT NULL DEFAULT TRUE,
    is_schedulable         BOOLEAN NOT NULL DEFAULT TRUE,
    status                 VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT instructor_profiles_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_brand_id ON instructor_profiles(brand_id);
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_brand_user_id ON instructor_profiles(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_instructor_profiles_brand_status ON instructor_profiles(brand_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_instructor_profiles_brand_user_unique
    ON instructor_profiles(brand_user_id);

CREATE TABLE IF NOT EXISTS instructor_course_templates (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id              BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    instructor_profile_id BIGINT NOT NULL REFERENCES instructor_profiles(id) ON DELETE CASCADE,
    course_id             BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_instructor_course_templates_brand_id ON instructor_course_templates(brand_id);
CREATE INDEX IF NOT EXISTS idx_instructor_course_templates_instructor_id
    ON instructor_course_templates(instructor_profile_id);
CREATE INDEX IF NOT EXISTS idx_instructor_course_templates_course_id ON instructor_course_templates(course_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_instructor_course_templates_unique
    ON instructor_course_templates(instructor_profile_id, course_id);

CREATE TABLE IF NOT EXISTS staff_weekly_unavailable_slots (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id      BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_user_id BIGINT NOT NULL REFERENCES brand_users(id) ON DELETE CASCADE,
    location_id   BIGINT REFERENCES locations(id) ON DELETE SET NULL,
    weekday       SMALLINT NOT NULL,
    start_time    TIME NOT NULL,
    end_time      TIME NOT NULL,
    reason        VARCHAR(500),
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT staff_weekly_unavailable_slots_weekday_valid CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT staff_weekly_unavailable_slots_time_valid CHECK (end_time > start_time),
    CONSTRAINT staff_weekly_unavailable_slots_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_staff_weekly_unavailable_slots_brand_id ON staff_weekly_unavailable_slots(brand_id);
CREATE INDEX IF NOT EXISTS idx_staff_weekly_unavailable_slots_brand_user_id
    ON staff_weekly_unavailable_slots(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_staff_weekly_unavailable_slots_location_id ON staff_weekly_unavailable_slots(location_id);

CREATE TABLE IF NOT EXISTS staff_unavailable_periods (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id      BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_user_id BIGINT NOT NULL REFERENCES brand_users(id) ON DELETE CASCADE,
    location_id   BIGINT REFERENCES locations(id) ON DELETE SET NULL,
    unavailable_type VARCHAR(30) NOT NULL DEFAULT 'temporary',
    starts_at     TIMESTAMPTZ NOT NULL,
    ends_at       TIMESTAMPTZ NOT NULL,
    reason        VARCHAR(500),
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT staff_unavailable_periods_type_valid CHECK (unavailable_type IN ('temporary', 'leave', 'suspended')),
    CONSTRAINT staff_unavailable_periods_period_valid CHECK (ends_at > starts_at),
    CONSTRAINT staff_unavailable_periods_status_valid CHECK (status IN ('active', 'cancelled'))
);
CREATE INDEX IF NOT EXISTS idx_staff_unavailable_periods_brand_id ON staff_unavailable_periods(brand_id);
CREATE INDEX IF NOT EXISTS idx_staff_unavailable_periods_brand_user_id ON staff_unavailable_periods(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_staff_unavailable_periods_location_id ON staff_unavailable_periods(location_id);
CREATE INDEX IF NOT EXISTS idx_staff_unavailable_periods_active_range
    ON staff_unavailable_periods(brand_user_id, starts_at, ends_at)
    WHERE status = 'active';

-- Learners.
CREATE TABLE IF NOT EXISTS learner_identities (
    id              BIGSERIAL PRIMARY KEY,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    wechat_open_id  VARCHAR(100) NOT NULL,
    wechat_union_id VARCHAR(100),
    phone           VARCHAR(20),
    nickname        VARCHAR(100),
    avatar_url      VARCHAR(500),
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT learner_identities_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_learner_identities_wechat_open_id ON learner_identities(wechat_open_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_learner_identities_wechat_union_id
    ON learner_identities(wechat_union_id)
    WHERE wechat_union_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_learner_identities_phone
    ON learner_identities(phone)
    WHERE phone IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_learner_identities_status ON learner_identities(status);

CREATE TABLE IF NOT EXISTS brand_learner_profiles (
    id                  BIGSERIAL PRIMARY KEY,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    brand_id            BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    learner_identity_id BIGINT NOT NULL REFERENCES learner_identities(id) ON DELETE CASCADE,
    primary_location_id BIGINT REFERENCES locations(id) ON DELETE SET NULL,
    learner_no          VARCHAR(50),
    nickname            VARCHAR(100),
    remark              VARCHAR(1000),
    status              VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT brand_learner_profiles_status_valid CHECK (status IN ('active', 'frozen', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_brand_learner_profiles_brand_id ON brand_learner_profiles(brand_id);
CREATE INDEX IF NOT EXISTS idx_brand_learner_profiles_identity_id ON brand_learner_profiles(learner_identity_id);
CREATE INDEX IF NOT EXISTS idx_brand_learner_profiles_primary_location_id ON brand_learner_profiles(primary_location_id);
CREATE INDEX IF NOT EXISTS idx_brand_learner_profiles_brand_status ON brand_learner_profiles(brand_id, status);
CREATE INDEX IF NOT EXISTS idx_brand_learner_profiles_deleted_at ON brand_learner_profiles(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_learner_profiles_brand_identity
    ON brand_learner_profiles(brand_id, learner_identity_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_learner_profiles_brand_learner_no
    ON brand_learner_profiles(brand_id, learner_no)
    WHERE learner_no IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS learner_tags (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id   BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name       VARCHAR(50) NOT NULL,
    color      VARCHAR(20),
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT learner_tags_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_learner_tags_brand_id ON learner_tags(brand_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_learner_tags_brand_name ON learner_tags(brand_id, name);

CREATE TABLE IF NOT EXISTS learner_tag_assignments (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    tag_id                   BIGINT NOT NULL REFERENCES learner_tags(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_learner_tag_assignments_brand_id ON learner_tag_assignments(brand_id);
CREATE INDEX IF NOT EXISTS idx_learner_tag_assignments_profile_id
    ON learner_tag_assignments(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_learner_tag_assignments_tag_id ON learner_tag_assignments(tag_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_learner_tag_assignments_unique
    ON learner_tag_assignments(brand_learner_profile_id, tag_id);

CREATE TABLE IF NOT EXISTS learner_staff_assignments (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    brand_user_id            BIGINT NOT NULL REFERENCES brand_users(id) ON DELETE CASCADE,
    location_id              BIGINT REFERENCES locations(id) ON DELETE SET NULL,
    relation_type            VARCHAR(30) NOT NULL,
    is_primary               BOOLEAN NOT NULL DEFAULT FALSE,
    status                   VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT learner_staff_assignments_type_valid CHECK (
        relation_type IN ('service_advisor', 'sales_advisor', 'primary_instructor', 'follow_up')
    ),
    CONSTRAINT learner_staff_assignments_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_learner_staff_assignments_brand_id ON learner_staff_assignments(brand_id);
CREATE INDEX IF NOT EXISTS idx_learner_staff_assignments_profile_id
    ON learner_staff_assignments(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_learner_staff_assignments_brand_user_id ON learner_staff_assignments(brand_user_id);
CREATE INDEX IF NOT EXISTS idx_learner_staff_assignments_location_id ON learner_staff_assignments(location_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_learner_staff_assignments_active_unique
    ON learner_staff_assignments(brand_learner_profile_id, brand_user_id, relation_type)
    WHERE status = 'active';

-- Course categories and availability.
CREATE TABLE IF NOT EXISTS course_categories (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id              BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name                  VARCHAR(100) NOT NULL,
    color                 VARCHAR(20),
    icon                  VARCHAR(100),
    sort_order            INT NOT NULL DEFAULT 0,
    show_in_mini_program  BOOLEAN NOT NULL DEFAULT TRUE,
    status                VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT course_categories_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_course_categories_brand_id ON course_categories(brand_id);
CREATE INDEX IF NOT EXISTS idx_course_categories_brand_status_sort
    ON course_categories(brand_id, status, sort_order);
CREATE UNIQUE INDEX IF NOT EXISTS idx_course_categories_brand_name ON course_categories(brand_id, name);

CREATE TABLE IF NOT EXISTS course_category_assignments (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id    BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    course_id   BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES course_categories(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_course_category_assignments_brand_id ON course_category_assignments(brand_id);
CREATE INDEX IF NOT EXISTS idx_course_category_assignments_course_id ON course_category_assignments(course_id);
CREATE INDEX IF NOT EXISTS idx_course_category_assignments_category_id ON course_category_assignments(category_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_course_category_assignments_unique
    ON course_category_assignments(course_id, category_id);

CREATE TABLE IF NOT EXISTS course_location_availability (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id    BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    course_id   BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    location_id BIGINT NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    is_available BOOLEAN NOT NULL DEFAULT TRUE,
    note        VARCHAR(500)
);
CREATE INDEX IF NOT EXISTS idx_course_location_availability_brand_id ON course_location_availability(brand_id);
CREATE INDEX IF NOT EXISTS idx_course_location_availability_course_id ON course_location_availability(course_id);
CREATE INDEX IF NOT EXISTS idx_course_location_availability_location_id ON course_location_availability(location_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_course_location_availability_unique
    ON course_location_availability(course_id, location_id);

-- Learner entitlements.
CREATE TABLE IF NOT EXISTS entitlement_products (
    id                           BIGSERIAL PRIMARY KEY,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                     BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name                         VARCHAR(100) NOT NULL,
    description                  VARCHAR(1000),
    product_type                 VARCHAR(30) NOT NULL,
    total_credits                INT,
    validity_days                INT NOT NULL,
    daily_booking_limit          INT,
    weekly_booking_limit         INT,
    monthly_booking_limit        INT,
    concurrent_booking_limit     INT,
    location_scope               VARCHAR(20) NOT NULL DEFAULT 'all',
    course_scope                 VARCHAR(20) NOT NULL DEFAULT 'all',
    status                       VARCHAR(20) NOT NULL DEFAULT 'active',
    CONSTRAINT entitlement_products_type_valid CHECK (
        product_type IN ('class_pack', 'trial_pack', 'membership_card')
    ),
    CONSTRAINT entitlement_products_credit_model_valid CHECK (
        (product_type IN ('class_pack', 'trial_pack') AND total_credits IS NOT NULL AND total_credits > 0)
        OR (product_type = 'membership_card' AND (total_credits IS NULL OR total_credits >= 0))
    ),
    CONSTRAINT entitlement_products_validity_positive CHECK (validity_days > 0),
    CONSTRAINT entitlement_products_limits_positive CHECK (
        (daily_booking_limit IS NULL OR daily_booking_limit > 0)
        AND (weekly_booking_limit IS NULL OR weekly_booking_limit > 0)
        AND (monthly_booking_limit IS NULL OR monthly_booking_limit > 0)
        AND (concurrent_booking_limit IS NULL OR concurrent_booking_limit > 0)
    ),
    CONSTRAINT entitlement_products_scope_valid CHECK (
        location_scope IN ('all', 'specific')
        AND course_scope IN ('all', 'specific')
    ),
    CONSTRAINT entitlement_products_status_valid CHECK (status IN ('active', 'inactive'))
);
CREATE INDEX IF NOT EXISTS idx_entitlement_products_brand_id ON entitlement_products(brand_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_products_brand_status ON entitlement_products(brand_id, status);
CREATE INDEX IF NOT EXISTS idx_entitlement_products_brand_type ON entitlement_products(brand_id, product_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlement_products_brand_name_active
    ON entitlement_products(brand_id, name)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS entitlement_product_locations (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id   BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES entitlement_products(id) ON DELETE CASCADE,
    location_id BIGINT NOT NULL REFERENCES locations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_entitlement_product_locations_brand_id ON entitlement_product_locations(brand_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_product_locations_product_id ON entitlement_product_locations(product_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_product_locations_location_id ON entitlement_product_locations(location_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlement_product_locations_unique
    ON entitlement_product_locations(product_id, location_id);

CREATE TABLE IF NOT EXISTS entitlement_product_courses (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id   BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES entitlement_products(id) ON DELETE CASCADE,
    course_id  BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_entitlement_product_courses_brand_id ON entitlement_product_courses(brand_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_product_courses_product_id ON entitlement_product_courses(product_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_product_courses_course_id ON entitlement_product_courses(course_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlement_product_courses_unique
    ON entitlement_product_courses(product_id, course_id);

CREATE TABLE IF NOT EXISTS learner_entitlements (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    product_id               BIGINT NOT NULL REFERENCES entitlement_products(id) ON DELETE RESTRICT,
    status                   VARCHAR(20) NOT NULL DEFAULT 'active',
    total_credits            INT,
    remaining_credits        INT,
    locked_credits           INT NOT NULL DEFAULT 0,
    consumed_credits         INT NOT NULL DEFAULT 0,
    starts_at                TIMESTAMPTZ NOT NULL,
    expires_at               TIMESTAMPTZ NOT NULL,
    granted_by               BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    remark                   VARCHAR(1000),
    CONSTRAINT learner_entitlements_status_valid CHECK (
        status IN ('active', 'expired', 'depleted', 'frozen', 'cancelled')
    ),
    CONSTRAINT learner_entitlements_period_valid CHECK (expires_at > starts_at),
    CONSTRAINT learner_entitlements_credit_non_negative CHECK (
        (total_credits IS NULL OR total_credits >= 0)
        AND (remaining_credits IS NULL OR remaining_credits >= 0)
        AND locked_credits >= 0
        AND consumed_credits >= 0
    )
);
CREATE INDEX IF NOT EXISTS idx_learner_entitlements_brand_id ON learner_entitlements(brand_id);
CREATE INDEX IF NOT EXISTS idx_learner_entitlements_profile_id ON learner_entitlements(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_learner_entitlements_product_id ON learner_entitlements(product_id);
CREATE INDEX IF NOT EXISTS idx_learner_entitlements_granted_by ON learner_entitlements(granted_by);
CREATE INDEX IF NOT EXISTS idx_learner_entitlements_usable
    ON learner_entitlements(brand_learner_profile_id, status, expires_at)
    WHERE status = 'active';

-- Booking policies and schedules.
CREATE TABLE IF NOT EXISTS brand_booking_policies (
    id                           BIGSERIAL PRIMARY KEY,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                     BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    location_id                  BIGINT REFERENCES locations(id) ON DELETE CASCADE,
    book_ahead_max_minutes       INT,
    book_ahead_min_minutes       INT NOT NULL DEFAULT 0,
    cancel_deadline_minutes      INT NOT NULL DEFAULT 0,
    release_on_cancel            BOOLEAN NOT NULL DEFAULT TRUE,
    no_show_consumes_entitlement BOOLEAN NOT NULL DEFAULT FALSE,
    daily_booking_limit          INT,
    weekly_booking_limit         INT,
    concurrent_booking_limit     INT,
    allow_waitlist               BOOLEAN NOT NULL DEFAULT TRUE,
    waitlist_limit               INT NOT NULL DEFAULT 0,
    CONSTRAINT brand_booking_policies_minutes_valid CHECK (
        (book_ahead_max_minutes IS NULL OR book_ahead_max_minutes >= 0)
        AND book_ahead_min_minutes >= 0
        AND cancel_deadline_minutes >= 0
    ),
    CONSTRAINT brand_booking_policies_limits_valid CHECK (
        (daily_booking_limit IS NULL OR daily_booking_limit > 0)
        AND (weekly_booking_limit IS NULL OR weekly_booking_limit > 0)
        AND (concurrent_booking_limit IS NULL OR concurrent_booking_limit > 0)
        AND waitlist_limit >= 0
    )
);
CREATE INDEX IF NOT EXISTS idx_brand_booking_policies_brand_id ON brand_booking_policies(brand_id);
CREATE INDEX IF NOT EXISTS idx_brand_booking_policies_location_id ON brand_booking_policies(location_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_booking_policies_brand_default
    ON brand_booking_policies(brand_id)
    WHERE location_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_brand_booking_policies_brand_location
    ON brand_booking_policies(brand_id, location_id)
    WHERE location_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS recurring_schedules (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id              BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    location_id           BIGINT NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    location_resource_id  BIGINT REFERENCES location_resources(id) ON DELETE SET NULL,
    course_id             BIGINT NOT NULL REFERENCES courses(id) ON DELETE RESTRICT,
    instructor_profile_id BIGINT NOT NULL REFERENCES instructor_profiles(id) ON DELETE RESTRICT,
    start_date            DATE NOT NULL,
    end_date              DATE,
    repeat_weeks          INT,
    start_time            TIME NOT NULL,
    duration_min          INT NOT NULL,
    capacity              INT NOT NULL,
    status                VARCHAR(20) NOT NULL DEFAULT 'active',
    created_by            BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    CONSTRAINT recurring_schedules_period_valid CHECK (end_date IS NULL OR end_date >= start_date),
    CONSTRAINT recurring_schedules_repeat_weeks_valid CHECK (repeat_weeks IS NULL OR repeat_weeks > 0),
    CONSTRAINT recurring_schedules_duration_positive CHECK (duration_min > 0),
    CONSTRAINT recurring_schedules_capacity_positive CHECK (capacity > 0),
    CONSTRAINT recurring_schedules_status_valid CHECK (status IN ('active', 'cancelled', 'completed'))
);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_brand_id ON recurring_schedules(brand_id);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_location_id ON recurring_schedules(location_id);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_resource_id ON recurring_schedules(location_resource_id);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_course_id ON recurring_schedules(course_id);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_instructor_id ON recurring_schedules(instructor_profile_id);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_created_by ON recurring_schedules(created_by);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_status_start ON recurring_schedules(status, start_date);

CREATE TABLE IF NOT EXISTS recurring_schedule_weekdays (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recurring_schedule_id BIGINT NOT NULL REFERENCES recurring_schedules(id) ON DELETE CASCADE,
    weekday               SMALLINT NOT NULL,
    CONSTRAINT recurring_schedule_weekdays_weekday_valid CHECK (weekday BETWEEN 0 AND 6)
);
CREATE INDEX IF NOT EXISTS idx_recurring_schedule_weekdays_schedule_id
    ON recurring_schedule_weekdays(recurring_schedule_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_recurring_schedule_weekdays_unique
    ON recurring_schedule_weekdays(recurring_schedule_id, weekday);

CREATE TABLE IF NOT EXISTS class_sessions (
    id                           BIGSERIAL PRIMARY KEY,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                     BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    location_id                  BIGINT NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    location_resource_id         BIGINT REFERENCES location_resources(id) ON DELETE SET NULL,
    course_id                    BIGINT NOT NULL REFERENCES courses(id) ON DELETE RESTRICT,
    instructor_profile_id        BIGINT NOT NULL REFERENCES instructor_profiles(id) ON DELETE RESTRICT,
    recurring_schedule_id        BIGINT REFERENCES recurring_schedules(id) ON DELETE SET NULL,
    starts_at                    TIMESTAMPTZ NOT NULL,
    ends_at                      TIMESTAMPTZ NOT NULL,
    capacity                     INT NOT NULL,
    booked_count                 INT NOT NULL DEFAULT 0,
    waitlist_limit               INT NOT NULL DEFAULT 0,
    status                       VARCHAR(20) NOT NULL DEFAULT 'draft',
    cancel_reason                VARCHAR(1000),
    created_by                   BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    CONSTRAINT class_sessions_period_valid CHECK (ends_at > starts_at),
    CONSTRAINT class_sessions_capacity_valid CHECK (capacity > 0 AND booked_count >= 0 AND booked_count <= capacity),
    CONSTRAINT class_sessions_waitlist_limit_valid CHECK (waitlist_limit >= 0),
    CONSTRAINT class_sessions_status_valid CHECK (
        status IN ('draft', 'scheduled', 'in_progress', 'completed', 'cancelled')
    ),
    CONSTRAINT class_sessions_instructor_no_overlap EXCLUDE USING gist (
        instructor_profile_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status IN ('scheduled', 'in_progress')),
    CONSTRAINT class_sessions_resource_no_overlap EXCLUDE USING gist (
        location_resource_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (location_resource_id IS NOT NULL AND status IN ('scheduled', 'in_progress'))
);
CREATE INDEX IF NOT EXISTS idx_class_sessions_brand_id ON class_sessions(brand_id);
CREATE INDEX IF NOT EXISTS idx_class_sessions_location_id ON class_sessions(location_id);
CREATE INDEX IF NOT EXISTS idx_class_sessions_resource_id ON class_sessions(location_resource_id);
CREATE INDEX IF NOT EXISTS idx_class_sessions_course_id ON class_sessions(course_id);
CREATE INDEX IF NOT EXISTS idx_class_sessions_instructor_id ON class_sessions(instructor_profile_id);
CREATE INDEX IF NOT EXISTS idx_class_sessions_recurring_schedule_id ON class_sessions(recurring_schedule_id);
CREATE INDEX IF NOT EXISTS idx_class_sessions_created_by ON class_sessions(created_by);
CREATE INDEX IF NOT EXISTS idx_class_sessions_schedule_lookup
    ON class_sessions(brand_id, location_id, status, starts_at);
CREATE INDEX IF NOT EXISTS idx_class_sessions_instructor_calendar
    ON class_sessions(instructor_profile_id, starts_at)
    WHERE status IN ('scheduled', 'in_progress', 'completed');

CREATE TABLE IF NOT EXISTS class_session_policy_overrides (
    id                           BIGSERIAL PRIMARY KEY,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                     BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    class_session_id             BIGINT NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    allow_cancel                 BOOLEAN,
    cancel_deadline_minutes      INT,
    release_on_cancel            BOOLEAN,
    no_show_consumes_entitlement BOOLEAN,
    allow_waitlist               BOOLEAN,
    waitlist_limit               INT,
    CONSTRAINT class_session_policy_overrides_values_valid CHECK (
        (cancel_deadline_minutes IS NULL OR cancel_deadline_minutes >= 0)
        AND (waitlist_limit IS NULL OR waitlist_limit >= 0)
    )
);
CREATE INDEX IF NOT EXISTS idx_class_session_policy_overrides_brand_id ON class_session_policy_overrides(brand_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_class_session_policy_overrides_session_id
    ON class_session_policy_overrides(class_session_id);

-- Bookings, waitlist, attendance, and entitlement accounting.
CREATE TABLE IF NOT EXISTS bookings (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    class_session_id         BIGINT NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    source                   VARCHAR(30) NOT NULL,
    status                   VARCHAR(30) NOT NULL DEFAULT 'booked',
    booked_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cancelled_at             TIMESTAMPTZ,
    cancelled_by             BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    cancel_source            VARCHAR(30),
    cancel_reason            VARCHAR(1000),
    assisted_by              BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    requires_entitlement_fix BOOLEAN NOT NULL DEFAULT FALSE,
    no_entitlement_reason    VARCHAR(1000),
    CONSTRAINT bookings_source_valid CHECK (source IN ('learner_self_service', 'staff_assisted', 'waitlist_promotion')),
    CONSTRAINT bookings_status_valid CHECK (
        status IN ('booked', 'cancelled', 'attended', 'pending_no_show', 'no_show')
    ),
    CONSTRAINT bookings_cancel_source_valid CHECK (
        cancel_source IS NULL OR cancel_source IN ('learner', 'staff', 'session_cancelled', 'system')
    ),
    CONSTRAINT bookings_assisted_reason_valid CHECK (
        requires_entitlement_fix = FALSE OR no_entitlement_reason IS NOT NULL
    )
);
CREATE INDEX IF NOT EXISTS idx_bookings_brand_id ON bookings(brand_id);
CREATE INDEX IF NOT EXISTS idx_bookings_class_session_id ON bookings(class_session_id);
CREATE INDEX IF NOT EXISTS idx_bookings_profile_id ON bookings(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_bookings_cancelled_by ON bookings(cancelled_by);
CREATE INDEX IF NOT EXISTS idx_bookings_assisted_by ON bookings(assisted_by);
CREATE INDEX IF NOT EXISTS idx_bookings_session_status ON bookings(class_session_id, status);
CREATE INDEX IF NOT EXISTS idx_bookings_profile_status_booked_at
    ON bookings(brand_learner_profile_id, status, booked_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_session_learner_unique
    ON bookings(class_session_id, brand_learner_profile_id);

CREATE TABLE IF NOT EXISTS waitlist_entries (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    class_session_id         BIGINT NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    position                 INT NOT NULL,
    status                   VARCHAR(30) NOT NULL DEFAULT 'waiting',
    promoted_booking_id      BIGINT REFERENCES bookings(id) ON DELETE SET NULL,
    skipped_reason           VARCHAR(1000),
    operated_by              BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    CONSTRAINT waitlist_entries_position_positive CHECK (position > 0),
    CONSTRAINT waitlist_entries_status_valid CHECK (
        status IN ('waiting', 'eligible_to_promote', 'promoted', 'cancelled', 'skipped')
    )
);
CREATE INDEX IF NOT EXISTS idx_waitlist_entries_brand_id ON waitlist_entries(brand_id);
CREATE INDEX IF NOT EXISTS idx_waitlist_entries_class_session_id ON waitlist_entries(class_session_id);
CREATE INDEX IF NOT EXISTS idx_waitlist_entries_profile_id ON waitlist_entries(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_waitlist_entries_promoted_booking_id ON waitlist_entries(promoted_booking_id);
CREATE INDEX IF NOT EXISTS idx_waitlist_entries_operated_by ON waitlist_entries(operated_by);
CREATE INDEX IF NOT EXISTS idx_waitlist_entries_session_queue
    ON waitlist_entries(class_session_id, status, position)
    WHERE status IN ('waiting', 'eligible_to_promote');
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlist_entries_session_learner_unique
    ON waitlist_entries(class_session_id, brand_learner_profile_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlist_entries_session_position_active
    ON waitlist_entries(class_session_id, position)
    WHERE status IN ('waiting', 'eligible_to_promote');

CREATE TABLE IF NOT EXISTS entitlement_holds (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    booking_id               BIGINT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    learner_entitlement_id   BIGINT NOT NULL REFERENCES learner_entitlements(id) ON DELETE RESTRICT,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    credits                  INT NOT NULL DEFAULT 1,
    status                   VARCHAR(20) NOT NULL DEFAULT 'held',
    held_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at              TIMESTAMPTZ,
    consumed_at              TIMESTAMPTZ,
    CONSTRAINT entitlement_holds_credits_positive CHECK (credits > 0),
    CONSTRAINT entitlement_holds_status_valid CHECK (status IN ('held', 'released', 'consumed'))
);
CREATE INDEX IF NOT EXISTS idx_entitlement_holds_brand_id ON entitlement_holds(brand_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_holds_booking_id ON entitlement_holds(booking_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_holds_entitlement_id ON entitlement_holds(learner_entitlement_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_holds_profile_id ON entitlement_holds(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_holds_status ON entitlement_holds(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlement_holds_booking_unique ON entitlement_holds(booking_id);

CREATE TABLE IF NOT EXISTS attendance_records (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    booking_id               BIGINT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    class_session_id         BIGINT NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    marked_by                BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    attended_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note                     VARCHAR(1000)
);
CREATE INDEX IF NOT EXISTS idx_attendance_records_brand_id ON attendance_records(brand_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_booking_id ON attendance_records(booking_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_class_session_id ON attendance_records(class_session_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_profile_id ON attendance_records(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_marked_by ON attendance_records(marked_by);
CREATE UNIQUE INDEX IF NOT EXISTS idx_attendance_records_booking_unique ON attendance_records(booking_id);

CREATE TABLE IF NOT EXISTS entitlement_consumptions (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    entitlement_hold_id      BIGINT REFERENCES entitlement_holds(id) ON DELETE SET NULL,
    learner_entitlement_id   BIGINT NOT NULL REFERENCES learner_entitlements(id) ON DELETE RESTRICT,
    booking_id               BIGINT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    attendance_id            BIGINT REFERENCES attendance_records(id) ON DELETE SET NULL,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    credits                  INT NOT NULL DEFAULT 1,
    consumption_type         VARCHAR(30) NOT NULL,
    consumed_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    operated_by              BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    note                     VARCHAR(1000),
    CONSTRAINT entitlement_consumptions_credits_positive CHECK (credits > 0),
    CONSTRAINT entitlement_consumptions_type_valid CHECK (
        consumption_type IN ('attendance', 'no_show', 'manual')
    )
);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_brand_id ON entitlement_consumptions(brand_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_hold_id ON entitlement_consumptions(entitlement_hold_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_entitlement_id
    ON entitlement_consumptions(learner_entitlement_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_booking_id ON entitlement_consumptions(booking_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_attendance_id ON entitlement_consumptions(attendance_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_profile_id ON entitlement_consumptions(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_consumptions_operated_by ON entitlement_consumptions(operated_by);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlement_consumptions_hold_unique
    ON entitlement_consumptions(entitlement_hold_id)
    WHERE entitlement_hold_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS session_records (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    class_session_id         BIGINT NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    booking_id               BIGINT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    attendance_id            BIGINT REFERENCES attendance_records(id) ON DELETE SET NULL,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    instructor_profile_id    BIGINT REFERENCES instructor_profiles(id) ON DELETE SET NULL,
    record_type              VARCHAR(30) NOT NULL,
    note                     VARCHAR(2000),
    metadata                 JSONB NOT NULL DEFAULT '{}'::JSONB,
    CONSTRAINT session_records_type_valid CHECK (record_type IN ('attendance', 'no_show', 'manual'))
);
CREATE INDEX IF NOT EXISTS idx_session_records_brand_id ON session_records(brand_id);
CREATE INDEX IF NOT EXISTS idx_session_records_class_session_id ON session_records(class_session_id);
CREATE INDEX IF NOT EXISTS idx_session_records_booking_id ON session_records(booking_id);
CREATE INDEX IF NOT EXISTS idx_session_records_attendance_id ON session_records(attendance_id);
CREATE INDEX IF NOT EXISTS idx_session_records_profile_id ON session_records(brand_learner_profile_id);
CREATE INDEX IF NOT EXISTS idx_session_records_instructor_id ON session_records(instructor_profile_id);
CREATE INDEX IF NOT EXISTS idx_session_records_profile_created_at
    ON session_records(brand_learner_profile_id, created_at DESC);

CREATE TABLE IF NOT EXISTS entitlement_transactions (
    id                       BIGSERIAL PRIMARY KEY,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id                 BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    learner_entitlement_id   BIGINT NOT NULL REFERENCES learner_entitlements(id) ON DELETE CASCADE,
    brand_learner_profile_id BIGINT NOT NULL REFERENCES brand_learner_profiles(id) ON DELETE CASCADE,
    booking_id               BIGINT REFERENCES bookings(id) ON DELETE SET NULL,
    hold_id                  BIGINT REFERENCES entitlement_holds(id) ON DELETE SET NULL,
    consumption_id           BIGINT REFERENCES entitlement_consumptions(id) ON DELETE SET NULL,
    action                   VARCHAR(30) NOT NULL,
    delta_credits            INT NOT NULL,
    balance_after            INT,
    note                     VARCHAR(1000),
    operated_by              BIGINT REFERENCES brand_users(id) ON DELETE SET NULL,
    CONSTRAINT entitlement_transactions_action_valid CHECK (
        action IN ('grant', 'hold', 'release', 'consume', 'no_show_consume', 'manual_adjust')
    )
);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_brand_id ON entitlement_transactions(brand_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_entitlement_id
    ON entitlement_transactions(learner_entitlement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_profile_id
    ON entitlement_transactions(brand_learner_profile_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_booking_id ON entitlement_transactions(booking_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_hold_id ON entitlement_transactions(hold_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_consumption_id ON entitlement_transactions(consumption_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_transactions_operated_by ON entitlement_transactions(operated_by);

-- Notifications, audit, and settings.
CREATE TABLE IF NOT EXISTS notifications (
    id                  BIGSERIAL PRIMARY KEY,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id            BIGINT REFERENCES brands(id) ON DELETE CASCADE,
    recipient_user_type VARCHAR(30) NOT NULL,
    recipient_id        BIGINT NOT NULL,
    channel             VARCHAR(30) NOT NULL,
    event_type          VARCHAR(80) NOT NULL,
    title               VARCHAR(200) NOT NULL,
    body                VARCHAR(2000),
    status              VARCHAR(30) NOT NULL DEFAULT 'pending',
    related_type        VARCHAR(80),
    related_id          BIGINT,
    sent_at             TIMESTAMPTZ,
    read_at             TIMESTAMPTZ,
    error_message       VARCHAR(1000),
    CONSTRAINT notifications_recipient_type_valid CHECK (
        recipient_user_type IN ('platform_admin', 'brand_user', 'learner')
    ),
    CONSTRAINT notifications_channel_valid CHECK (channel IN ('in_app', 'wechat_subscribe')),
    CONSTRAINT notifications_status_valid CHECK (status IN ('pending', 'sent', 'failed', 'read'))
);
CREATE INDEX IF NOT EXISTS idx_notifications_brand_id ON notifications(brand_id);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_lookup
    ON notifications(recipient_user_type, recipient_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_pending
    ON notifications(channel, created_at)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_notifications_related ON notifications(related_type, related_id);

CREATE TABLE IF NOT EXISTS operation_logs (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    brand_id    BIGINT REFERENCES brands(id) ON DELETE SET NULL,
    actor_type  VARCHAR(30) NOT NULL,
    actor_id    BIGINT,
    action      VARCHAR(100) NOT NULL,
    target_type VARCHAR(80),
    target_id   BIGINT,
    reason      VARCHAR(1000),
    metadata    JSONB NOT NULL DEFAULT '{}'::JSONB,
    CONSTRAINT operation_logs_actor_type_valid CHECK (
        actor_type IN ('platform_admin', 'brand_user', 'learner', 'system')
    )
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_brand_id ON operation_logs(brand_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_brand_action_created_at
    ON operation_logs(brand_id, action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_logs_actor ON operation_logs(actor_type, actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_logs_target ON operation_logs(target_type, target_id);

CREATE TABLE IF NOT EXISTS system_settings (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scope         VARCHAR(20) NOT NULL DEFAULT 'platform',
    brand_id      BIGINT REFERENCES brands(id) ON DELETE CASCADE,
    setting_key   VARCHAR(100) NOT NULL,
    setting_value JSONB NOT NULL DEFAULT '{}'::JSONB,
    description   VARCHAR(500),
    CONSTRAINT system_settings_scope_valid CHECK (scope IN ('platform', 'brand')),
    CONSTRAINT system_settings_scope_brand_valid CHECK (
        (scope = 'platform' AND brand_id IS NULL)
        OR (scope = 'brand' AND brand_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_system_settings_brand_id ON system_settings(brand_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_system_settings_platform_key
    ON system_settings(setting_key)
    WHERE brand_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_system_settings_brand_key
    ON system_settings(brand_id, setting_key)
    WHERE brand_id IS NOT NULL;

-- Default permission and role-template seed data.
INSERT INTO permissions (code, domain, action, name, description)
VALUES
    ('brand.view', 'brand', 'view', '查看品牌设置', '查看品牌基础设置'),
    ('brand.edit', 'brand', 'edit', '编辑品牌设置', '编辑品牌基础设置'),
    ('location.view', 'location', 'view', '查看 Location', '查看 Location 与资源'),
    ('location.manage', 'location', 'manage', '管理 Location', '新增、编辑、停用 Location 与资源'),
    ('staff.view', 'staff', 'view', '查看员工', '查看员工与权限'),
    ('staff.manage', 'staff', 'manage', '管理员工', '新增、编辑、停用员工和权限'),
    ('learner.view', 'learner', 'view', '查看学员', '查看学员资料'),
    ('learner.manage', 'learner', 'manage', '管理学员', '新增、编辑、冻结学员'),
    ('entitlement.view', 'entitlement', 'view', '查看权益', '查看权益和流水'),
    ('entitlement.manage', 'entitlement', 'manage', '管理权益', '开通和管理权益模板'),
    ('entitlement.adjust', 'entitlement', 'adjust', '人工调整权益', '手动调整学员权益'),
    ('course.view', 'course', 'view', '查看课程', '查看课程分类和模板'),
    ('course.manage', 'course', 'manage', '管理课程', '管理课程分类和课程模板'),
    ('schedule.view', 'schedule', 'view', '查看排课', '查看课表和场次'),
    ('schedule.manage', 'schedule', 'manage', '管理排课', '创建、编辑和取消场次'),
    ('booking.view', 'booking', 'view', '查看预约', '查看预约与候补'),
    ('booking.create_assisted', 'booking', 'create_assisted', '员工代预约', '员工代学员预约'),
    ('booking.cancel', 'booking', 'cancel', '取消预约', '员工代取消预约'),
    ('attendance.view', 'attendance', 'view', '查看签到', '查看签到和爽约'),
    ('attendance.mark', 'attendance', 'mark', '手动签到', '手动标记到课'),
    ('attendance.no_show_confirm', 'attendance', 'no_show_confirm', '确认爽约', '确认并处理爽约'),
    ('report.view_basic', 'report', 'view_basic', '查看基础报表', '查看基础运营报表'),
    ('report.view_finance', 'report', 'view_finance', '查看财务报表', '查看财务相关报表'),
    ('payment.view', 'payment', 'view', '查看支付', '查看支付订单和流水'),
    ('subscription.manage', 'subscription', 'manage', '管理订阅', '管理套餐订阅和额度')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_templates (code, name, scope_type, description)
VALUES
    ('brand_owner', '品牌负责人', 'brand', '品牌最高权限负责人'),
    ('brand_admin', '品牌管理员', 'brand', '品牌日常运营管理员'),
    ('course_operator', '课程运营', 'brand', '课程、排课和预约运营'),
    ('finance_support', '财务/售后', 'brand', '财务、售后和权益处理'),
    ('location_manager', '店长', 'location', '指定 Location 负责人'),
    ('receptionist', '前台', 'location', '预约、签到和现场服务'),
    ('instructor', 'Instructor', 'location', '授课员工'),
    ('location_assistant', '门店协助', 'location', '门店辅助角色')
ON CONFLICT (code) DO NOTHING;

WITH mappings(template_code, permission_code) AS (
    VALUES
        ('brand_owner', 'brand.view'), ('brand_owner', 'brand.edit'), ('brand_owner', 'location.view'), ('brand_owner', 'location.manage'),
        ('brand_owner', 'staff.view'), ('brand_owner', 'staff.manage'), ('brand_owner', 'learner.view'), ('brand_owner', 'learner.manage'),
        ('brand_owner', 'entitlement.view'), ('brand_owner', 'entitlement.manage'), ('brand_owner', 'entitlement.adjust'),
        ('brand_owner', 'course.view'), ('brand_owner', 'course.manage'), ('brand_owner', 'schedule.view'), ('brand_owner', 'schedule.manage'),
        ('brand_owner', 'booking.view'), ('brand_owner', 'booking.create_assisted'), ('brand_owner', 'booking.cancel'),
        ('brand_owner', 'attendance.view'), ('brand_owner', 'attendance.mark'), ('brand_owner', 'attendance.no_show_confirm'),
        ('brand_owner', 'report.view_basic'), ('brand_owner', 'report.view_finance'), ('brand_owner', 'payment.view'), ('brand_owner', 'subscription.manage'),
        ('brand_admin', 'brand.view'), ('brand_admin', 'brand.edit'), ('brand_admin', 'location.view'), ('brand_admin', 'location.manage'),
        ('brand_admin', 'staff.view'), ('brand_admin', 'staff.manage'), ('brand_admin', 'learner.view'), ('brand_admin', 'learner.manage'),
        ('brand_admin', 'entitlement.view'), ('brand_admin', 'entitlement.manage'), ('brand_admin', 'course.view'), ('brand_admin', 'course.manage'),
        ('brand_admin', 'schedule.view'), ('brand_admin', 'schedule.manage'), ('brand_admin', 'booking.view'), ('brand_admin', 'booking.create_assisted'),
        ('brand_admin', 'booking.cancel'), ('brand_admin', 'attendance.view'), ('brand_admin', 'attendance.mark'), ('brand_admin', 'attendance.no_show_confirm'),
        ('brand_admin', 'report.view_basic'), ('brand_admin', 'report.view_finance'), ('brand_admin', 'payment.view'),
        ('course_operator', 'brand.view'), ('course_operator', 'location.view'), ('course_operator', 'staff.view'), ('course_operator', 'learner.view'),
        ('course_operator', 'course.view'), ('course_operator', 'course.manage'), ('course_operator', 'schedule.view'), ('course_operator', 'schedule.manage'),
        ('course_operator', 'booking.view'), ('course_operator', 'report.view_basic'),
        ('finance_support', 'brand.view'), ('finance_support', 'location.view'), ('finance_support', 'learner.view'), ('finance_support', 'entitlement.view'),
        ('finance_support', 'entitlement.manage'), ('finance_support', 'course.view'), ('finance_support', 'schedule.view'), ('finance_support', 'booking.view'),
        ('finance_support', 'report.view_finance'), ('finance_support', 'payment.view'),
        ('location_manager', 'location.view'), ('location_manager', 'staff.view'), ('location_manager', 'learner.view'), ('location_manager', 'learner.manage'),
        ('location_manager', 'course.view'), ('location_manager', 'schedule.view'), ('location_manager', 'schedule.manage'), ('location_manager', 'booking.view'),
        ('location_manager', 'booking.create_assisted'), ('location_manager', 'booking.cancel'), ('location_manager', 'attendance.view'),
        ('location_manager', 'attendance.mark'), ('location_manager', 'attendance.no_show_confirm'), ('location_manager', 'report.view_basic'),
        ('receptionist', 'location.view'), ('receptionist', 'learner.view'), ('receptionist', 'learner.manage'), ('receptionist', 'course.view'),
        ('receptionist', 'schedule.view'), ('receptionist', 'booking.view'), ('receptionist', 'booking.create_assisted'), ('receptionist', 'booking.cancel'),
        ('receptionist', 'attendance.view'), ('receptionist', 'attendance.mark'), ('receptionist', 'attendance.no_show_confirm'),
        ('instructor', 'location.view'), ('instructor', 'learner.view'), ('instructor', 'course.view'), ('instructor', 'schedule.view'),
        ('instructor', 'booking.view'), ('instructor', 'attendance.view'), ('instructor', 'attendance.mark'),
        ('location_assistant', 'location.view'), ('location_assistant', 'learner.view'), ('location_assistant', 'course.view'), ('location_assistant', 'schedule.view'),
        ('location_assistant', 'booking.view'), ('location_assistant', 'attendance.view')
)
INSERT INTO role_template_permissions (template_id, permission_id)
SELECT rt.id, p.id
FROM mappings m
JOIN role_templates rt ON rt.code = m.template_code
JOIN permissions p ON p.code = m.permission_code
ON CONFLICT (template_id, permission_id) DO NOTHING;

INSERT INTO brand_roles (brand_id, template_id, code, name, scope_type, is_system, description)
SELECT b.id, rt.id, rt.code, rt.name, rt.scope_type, TRUE, rt.description
FROM brands b
CROSS JOIN role_templates rt
WHERE b.deleted_at IS NULL
ON CONFLICT (brand_id, code) DO NOTHING;

INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
SELECT br.brand_id, br.id, rtp.permission_id
FROM brand_roles br
JOIN role_templates rt ON rt.id = br.template_id
JOIN role_template_permissions rtp ON rtp.template_id = rt.id
ON CONFLICT (role_id, permission_id) DO NOTHING;
