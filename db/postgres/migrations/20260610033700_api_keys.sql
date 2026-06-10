-- +goose Up
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_token ON api_keys (token);

CREATE VIEW api_keys_view AS
SELECT
    id,
    name,
    token,
    created_at
FROM api_keys
WHERE revoked_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS api_keys_view;

DROP TABLE IF EXISTS api_keys;
