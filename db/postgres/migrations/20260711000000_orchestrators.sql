-- +goose Up
-- Orchestrators are a distinct resource from agents: one orchestrator fronts a
-- set of sub-agents and routes to them. We persist only what the external
-- orchestrator/create service returns that we can't cheaply re-derive:
-- routing_guide, persona, guardrail (same format as an agent), plus the
-- orchestrator_agent_id used to actually call it later.
--
-- Per-agent mapping stores ONLY tool_name + description. endpoint and can_act
-- are NOT stored: endpoint == the agent's template_id and can_act lives on the
-- agent row, so we fetch+rename from the agent at call time (no redundancy).
CREATE TABLE orchestrators (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT false,
    -- id of the orchestrator agent created upstream; used to call it later.
    orchestrator_agent_id TEXT NOT NULL DEFAULT '',
    routing_guide TEXT NOT NULL DEFAULT '',
    persona TEXT NOT NULL DEFAULT '',
    guardrail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE orchestrator_agents (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    orchestrator_id UUID NOT NULL
    REFERENCES orchestrators (id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    tool_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_orchestrator_agents_orchestrator_id
ON orchestrator_agents (orchestrator_id);

CREATE INDEX idx_orchestrator_agents_agent_id
ON orchestrator_agents (agent_id);

CREATE UNIQUE INDEX idx_orchestrator_agent_unique
ON orchestrator_agents (orchestrator_id, agent_id)
WHERE deleted_at IS null;

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
    updated_at
FROM orchestrators
WHERE deleted_at IS null;

CREATE VIEW orchestrator_agents_view AS
SELECT
    id,
    orchestrator_id,
    agent_id,
    tool_name,
    description,
    created_at,
    updated_at
FROM orchestrator_agents
WHERE deleted_at IS null;

-- +goose Down
DROP VIEW IF EXISTS orchestrator_agents_view;
DROP VIEW IF EXISTS orchestrators_view;

DROP TABLE IF EXISTS orchestrator_agents;
DROP TABLE IF EXISTS orchestrators;
