-- +goose Up
-- Bring orchestrators to parity with agents for outbound webhook config.
-- Same contract and same {key, type, value} shape as agents.webhook_*_fields:
-- "static" sends a fixed value (e.g. Authorization: Bearer xxx), "dynamic" is
-- supplied by the caller per request (400 if missing). Empty array = no auth.
ALTER TABLE orchestrators
    ADD COLUMN webhook_input_field TEXT NOT NULL DEFAULT 'chatInput',
    ADD COLUMN webhook_output_field TEXT NOT NULL DEFAULT 'output',
    ADD COLUMN webhook_body_fields JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN webhook_header_fields JSONB NOT NULL DEFAULT '[]';

CREATE OR REPLACE VIEW orchestrators_view AS
SELECT
    id,
    name,
    description,
    is_active,
    orchestrator_agent_id,
    routing_guide,
    persona,
    guardrail,
    created_at,
    updated_at,
    image,
    webhook_uri,
    webhook_input_field,
    webhook_output_field,
    webhook_body_fields,
    webhook_header_fields
FROM orchestrators
WHERE deleted_at IS null;

-- +goose Down
-- CREATE OR REPLACE VIEW cannot drop a column, so drop and recreate.
DROP VIEW IF EXISTS orchestrators_view;
CREATE VIEW orchestrators_view AS
SELECT
    id,
    name,
    description,
    is_active,
    orchestrator_agent_id,
    routing_guide,
    persona,
    guardrail,
    created_at,
    updated_at,
    image,
    webhook_uri
FROM orchestrators
WHERE deleted_at IS null;

ALTER TABLE orchestrators
    DROP COLUMN IF EXISTS webhook_input_field,
    DROP COLUMN IF EXISTS webhook_output_field,
    DROP COLUMN IF EXISTS webhook_body_fields,
    DROP COLUMN IF EXISTS webhook_header_fields;
