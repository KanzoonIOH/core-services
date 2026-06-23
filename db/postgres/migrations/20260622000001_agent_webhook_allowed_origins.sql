-- +goose Up
-- Empty array means allow all browser origins (backward compatible).
ALTER TABLE agents ADD COLUMN webhook_allowed_origins TEXT[] NOT NULL DEFAULT '{}';

CREATE OR REPLACE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    is_active,
    webhook_uri,
    webhook_allowed_ips,
    webhook_allowed_origins,
    tone,
    response_length,
    communication_style,
    created_at,
    updated_at
FROM agents
WHERE deleted_at IS null;

-- +goose Down
CREATE OR REPLACE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    is_active,
    webhook_uri,
    webhook_allowed_ips,
    tone,
    response_length,
    communication_style,
    created_at,
    updated_at
FROM agents
WHERE deleted_at IS null;

ALTER TABLE agents DROP COLUMN webhook_allowed_origins;
