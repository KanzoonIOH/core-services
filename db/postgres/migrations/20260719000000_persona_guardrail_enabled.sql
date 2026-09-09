-- +goose Up
-- Per-target switches for what the chat proxy puts in the outbound webhook
-- body: persona_enabled gates tone/length/style, guardrail_enabled gates the
-- systemPrompt object (and the orchestrator's own guardrail). Default true to
-- keep today's behaviour for existing rows.
ALTER TABLE agents
    ADD COLUMN persona_enabled BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN guardrail_enabled BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE orchestrators
    ADD COLUMN persona_enabled BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN guardrail_enabled BOOLEAN NOT NULL DEFAULT true;

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
    webhook_stream_enabled,
    persona_enabled,
    guardrail_enabled
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
    webhook_stream_enabled,
    persona_enabled,
    guardrail_enabled
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
    template_id,
    webhook_stream_enabled
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
    webhook_header_fields,
    webhook_stream_enabled
FROM orchestrators
WHERE deleted_at IS null;

ALTER TABLE agents
    DROP COLUMN IF EXISTS persona_enabled,
    DROP COLUMN IF EXISTS guardrail_enabled;
ALTER TABLE orchestrators
    DROP COLUMN IF EXISTS persona_enabled,
    DROP COLUMN IF EXISTS guardrail_enabled;
