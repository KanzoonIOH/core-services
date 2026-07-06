-- +goose Up
-- Per-agent HTTP headers attached to the outbound webhook request.
-- Same {key, type, value} shape as webhook_body_fields: "static" sends a fixed
-- value (e.g. Authorization: Bearer xxx for a secured agent), "dynamic" is
-- supplied by the caller per request. Empty array = open agent, no auth.
ALTER TABLE agents ADD COLUMN webhook_header_fields JSONB NOT NULL DEFAULT '[]';

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
    webhook_body_fields,
    webhook_header_fields
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
    webhook_output_field,
    webhook_body_fields
FROM agents
WHERE deleted_at IS null;

ALTER TABLE agents DROP COLUMN IF EXISTS webhook_header_fields;
