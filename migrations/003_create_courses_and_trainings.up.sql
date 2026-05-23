-- +migrate Up
CREATE TABLE courses (
    id BIGSERIAL PRIMARY KEY,
    brand_id BIGINT NOT NULL REFERENCES brands(id),
    title VARCHAR(200) NOT NULL,
    description VARCHAR(2000),
    cover_url VARCHAR(500),
    difficulty VARCHAR(20) NOT NULL,
    duration_min INTEGER NOT NULL,
    type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_courses_brand_id ON courses(brand_id);
CREATE INDEX idx_courses_title ON courses(title);
CREATE INDEX idx_courses_type ON courses(type);
CREATE INDEX idx_courses_status ON courses(status);

CREATE TABLE training_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES app_users(id),
    brand_id BIGINT NOT NULL REFERENCES brands(id),
    course_id BIGINT NOT NULL REFERENCES courses(id),
    duration_min INTEGER NOT NULL,
    calories DOUBLE PRECISION NOT NULL DEFAULT 0,
    notes VARCHAR(500),
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_training_records_user_id ON training_records(user_id);
CREATE INDEX idx_training_records_brand_id ON training_records(brand_id);
CREATE INDEX idx_training_records_course_id ON training_records(course_id);
CREATE INDEX idx_training_records_completed_at ON training_records(completed_at);

-- +migrate Down
DROP TABLE training_records;
DROP TABLE courses;
