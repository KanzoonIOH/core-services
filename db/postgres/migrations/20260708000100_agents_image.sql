-- +goose Up
-- image holds either a URL (avatar picture in object storage) or an emoji string.
ALTER TABLE agents ADD COLUMN image TEXT;

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
    image
FROM agents
WHERE deleted_at IS null;

-- +goose Down
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
    guardrail
FROM agents
WHERE deleted_at IS null;

ALTER TABLE agents DROP COLUMN IF EXISTS image;
