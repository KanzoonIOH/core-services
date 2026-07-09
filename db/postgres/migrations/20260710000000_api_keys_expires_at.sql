-- +goose Up
ALTER TABLE api_keys ADD COLUMN expires_at TIMESTAMPTZ;

-- Recreate the view to expose expires_at. Listing still shows expired keys so
-- users can see/revoke them; the auth lookup enforces expiry in its own query.
DROP VIEW IF EXISTS api_keys_view;
CREATE VIEW api_keys_view AS
SELECT
    id,
    name,
    token,
    expires_at,
    created_at
FROM api_keys
WHERE revoked_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS api_keys_view;
CREATE VIEW api_keys_view AS
SELECT
    id,
    name,
    token,
    created_at
FROM api_keys
WHERE revoked_at IS NULL;

ALTER TABLE api_keys DROP COLUMN expires_at;
