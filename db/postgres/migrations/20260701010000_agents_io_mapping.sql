-- +goose Up
-- Per-agent overrides for the webhook request/response field names so we can
-- support vendors other than n8n. Defaults preserve current n8n behaviour.
ALTER TABLE agents
    ADD COLUMN webhook_input_field TEXT NOT NULL DEFAULT 'chatInput',
    ADD COLUMN webhook_output_field TEXT NOT NULL DEFAULT 'output';

CREATE OR REPLACE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    type,
    is_active,
    webhook_uri,
    webhook_allowed_ips,
    webhook_allowed_origins,
    tone,
    response_length,
    communication_style,
    created_at,
    updated_at,
    milvus_collection,
    webhook_input_field,
    webhook_output_field
FROM agents
WHERE deleted_at IS null;

-- +goose Down
-- CREATE OR REPLACE VIEW cannot drop a column, so drop and recreate.
DROP VIEW IF EXISTS agents_view;
CREATE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    type,
    is_active,
    webhook_uri,
    webhook_allowed_ips,
    webhook_allowed_origins,
    tone,
    response_length,
    communication_style,
    created_at,
    updated_at,
    milvus_collection
FROM agents
WHERE deleted_at IS null;

ALTER TABLE agents
    DROP COLUMN IF EXISTS webhook_input_field,
    DROP COLUMN IF EXISTS webhook_output_field;
