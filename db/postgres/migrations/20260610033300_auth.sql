-- +goose Up
CREATE TYPE USER_ROLE AS ENUM (
    'PENDING',
    'VIEWER',
    'TECHNICAL',
    'ADMIN',
    'SUPERADMIN'
);

CREATE TYPE UPCOMING_CHANGES_TYPE AS ENUM (
    'EMAIL',
    'PASSWORD',
    'FORGOT_PASSWORD'
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    role USER_ROLE NOT NULL DEFAULT 'PENDING',
    hashed_password TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_users_email_unique ON users (
    lower(trim(email))
) WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX idx_users_username_unique ON users (
    lower(trim(username))
) WHERE deleted_at IS NULL;

CREATE TABLE upcoming_changes (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    token TEXT NOT NULL,
    type UPCOMING_CHANGES_TYPE NOT NULL,
    upcoming_value TEXT,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expired_at TIMESTAMPTZ NOT NULL DEFAULT now() + INTERVAL '1 hour',
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_upcoming_changes_user_id_unique
ON upcoming_changes (user_id, type)
WHERE revoked_at IS NULL;

CREATE VIEW users_view AS
SELECT
    id,
    name,
    username,
    email,
    role,
    created_at,
    updated_at
FROM users
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS users_view;

DROP TABLE IF EXISTS upcoming_changes;
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS UPCOMING_CHANGES_TYPE;
DROP TYPE IF EXISTS USER_ROLE;
