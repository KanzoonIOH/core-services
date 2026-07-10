-- +goose Up
-- webhook_uri: the upstream URL this orchestrator forwards chat requests to.
-- Same contract as agents.webhook_uri — the chat handler reads it to know
-- where to POST /stream the request.
ALTER TABLE orchestrators ADD COLUMN webhook_uri TEXT NOT NULL DEFAULT '';

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
    webhook_uri
FROM orchestrators
WHERE deleted_at IS null;

-- +goose Down
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
    image,
    created_at,
    updated_at
FROM orchestrators
WHERE deleted_at IS null;

ALTER TABLE orchestrators DROP COLUMN IF EXISTS webhook_uri;
