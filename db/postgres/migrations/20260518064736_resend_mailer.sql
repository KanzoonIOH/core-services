-- +goose Up
CREATE TYPE UPCOMING_CHANGES_TYPE AS ENUM (
    'email',
    'password',
    'forgot-password'
);

CREATE TABLE upcoming_changes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token TEXT NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    type UPCOMING_CHANGES_TYPE NOT NULL,
    upcoming_value TEXT,
    --
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expired_at TIMESTAMPTZ NOT NULL DEFAULT now() + INTERVAL '1 hour',
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_upcoming_changes_token ON upcoming_changes (token);
CREATE UNIQUE INDEX idx_upcoming_changes_user_id_unique
ON upcoming_changes (user_id, type)
WHERE revoked_at IS NULL;
-- +goose Down
DROP TABLE IF EXISTS upcoming_changes;
DROP TYPE IF EXISTS UPCOMING_CHANGES_TYPE;
