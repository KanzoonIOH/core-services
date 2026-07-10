-- +goose Up
-- image holds either a URL (avatar picture in object storage) or an emoji string,
-- same contract as agents.image.
ALTER TABLE orchestrators ADD COLUMN image TEXT;

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
    image
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
    created_at,
    updated_at
FROM orchestrators
WHERE deleted_at IS null;

ALTER TABLE orchestrators DROP COLUMN IF EXISTS image;
