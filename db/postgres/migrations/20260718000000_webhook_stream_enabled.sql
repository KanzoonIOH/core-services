-- +goose Up
-- Whether this target serves the streaming variant. The chat proxy assumes an
-- n8n-style sibling endpoint (webhook_uri + "/stream"); upstreams that only
-- expose the plain URL set this false and get the non-streaming path instead.
-- Defaults true to preserve today's behaviour for existing rows.
ALTER TABLE agents
    ADD COLUMN webhook_stream_enabled BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE orchestrators
    ADD COLUMN webhook_stream_enabled BOOLEAN NOT NULL DEFAULT true;

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
    template_id,
    webhook_stream_enabled
FROM agents
WHERE deleted_at IS null;

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
    webhook_header_fields,
    webhook_stream_enabled
FROM orchestrators
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
    image,
    can_act,
    template_id
FROM agents
WHERE deleted_at IS null;

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
    webhook_uri,
    webhook_input_field,
    webhook_output_field,
    webhook_body_fields,
    webhook_header_fields
FROM orchestrators
WHERE deleted_at IS null;

ALTER TABLE agents DROP COLUMN IF EXISTS webhook_stream_enabled;
ALTER TABLE orchestrators DROP COLUMN IF EXISTS webhook_stream_enabled;
