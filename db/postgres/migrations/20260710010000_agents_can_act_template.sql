-- +goose Up
-- can_act: whether the agent may take actions (bookings, escalations) or is
--   read-only (Q&A only). Mirrors the static template field into the DB so
--   runtime code doesn't need to know which template an agent came from.
-- template_id: the static template id ("product", "booking", ...) this agent
--   was created from. Empty for scratch-built agents.
ALTER TABLE agents
    ADD COLUMN can_act BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN template_id TEXT NOT NULL DEFAULT '';

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
    webhook_header_fields,
    guardrail,
    image,
    can_act,
    template_id
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
    webhook_body_fields,
    webhook_header_fields,
    guardrail,
    image
FROM agents
WHERE deleted_at IS null;

ALTER TABLE agents
    DROP COLUMN IF EXISTS can_act,
    DROP COLUMN IF EXISTS template_id;
