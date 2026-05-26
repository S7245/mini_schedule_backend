-- 000003_course_booking_schema.down.sql

DROP TABLE IF EXISTS entitlement_transactions;
DROP TABLE IF EXISTS session_records;
DROP TABLE IF EXISTS entitlement_consumptions;
DROP TABLE IF EXISTS attendance_records;
DROP TABLE IF EXISTS entitlement_holds;
DROP TABLE IF EXISTS waitlist_entries;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS class_session_policy_overrides;
DROP TABLE IF EXISTS class_sessions;
DROP TABLE IF EXISTS recurring_schedule_weekdays;
DROP TABLE IF EXISTS recurring_schedules;
DROP TABLE IF EXISTS brand_booking_policies;
DROP TABLE IF EXISTS learner_entitlements;
DROP TABLE IF EXISTS entitlement_product_courses;
DROP TABLE IF EXISTS entitlement_product_locations;
DROP TABLE IF EXISTS entitlement_products;
DROP TABLE IF EXISTS course_location_availability;
DROP TABLE IF EXISTS course_category_assignments;
DROP TABLE IF EXISTS course_categories;
DROP TABLE IF EXISTS learner_staff_assignments;
DROP TABLE IF EXISTS learner_tag_assignments;
DROP TABLE IF EXISTS learner_tags;
DROP TABLE IF EXISTS brand_learner_profiles;
DROP TABLE IF EXISTS learner_identities;
DROP TABLE IF EXISTS staff_unavailable_periods;
DROP TABLE IF EXISTS staff_weekly_unavailable_slots;
DROP TABLE IF EXISTS instructor_course_templates;
DROP TABLE IF EXISTS instructor_profiles;
DROP TABLE IF EXISTS brand_user_role_assignments;
DROP TABLE IF EXISTS staff_location_assignments;
DROP TABLE IF EXISTS brand_role_permissions;
DROP TABLE IF EXISTS brand_roles;
DROP TABLE IF EXISTS role_template_permissions;
DROP TABLE IF EXISTS role_templates;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS brand_onboarding_steps;
DROP TABLE IF EXISTS location_resources;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS payment_callback_logs;
DROP TABLE IF EXISTS payment_transactions;
DROP TABLE IF EXISTS brand_subscription_features;
DROP TABLE IF EXISTS brand_subscriptions;
DROP TABLE IF EXISTS saas_plan_orders;
DROP TABLE IF EXISTS saas_plan_features;
DROP TABLE IF EXISTS saas_plans;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS operation_logs;
DROP TABLE IF EXISTS system_settings;

ALTER TABLE courses
    DROP CONSTRAINT IF EXISTS courses_default_capacity_positive;

ALTER TABLE courses
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS show_in_mini_program,
    DROP COLUMN IF EXISTS level_label,
    DROP COLUMN IF EXISTS default_capacity;

ALTER TABLE brand_users
    DROP COLUMN IF EXISTS last_login_at,
    DROP COLUMN IF EXISTS is_owner,
    DROP COLUMN IF EXISTS avatar_url,
    DROP COLUMN IF EXISTS email;

ALTER TABLE brands
    DROP COLUMN IF EXISTS onboarding_completed_at,
    DROP COLUMN IF EXISTS onboarding_status,
    DROP COLUMN IF EXISTS industry_type,
    DROP COLUMN IF EXISTS contact_email,
    DROP COLUMN IF EXISTS brand_code;
