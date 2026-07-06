-- +goose Up
-- Per-agent extra fields injected into the outbound webhook JSON body.
-- Array of {key, type, value}: type "static" sends value as-is, type "dynamic"
-- must be supplied by the caller per request (enforced, 400 if missing).
ALTER TABLE agents ADD COLUMN webhook_body_fields JSONB NOT NULL DEFAULT '[]';

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
    webhook_output_field,
    webhook_body_fields
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
    milvus_collection,
    webhook_input_field,
    webhook_output_field
FROM agents
WHERE deleted_at IS null;

ALTER TABLE agents DROP COLUMN IF EXISTS webhook_body_fields;
