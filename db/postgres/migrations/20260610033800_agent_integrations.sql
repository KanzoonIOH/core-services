-- +goose Up
CREATE TABLE agent_knowledges (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    is_active_prod BOOLEAN NOT NULL DEFAULT false,
    is_active_dev BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'pending',
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    knowledge_id UUID NOT NULL REFERENCES knowledges (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT agent_knowledges_status_check
    CHECK (status IN ('pending', 'completed', 'failed'))
);

CREATE INDEX idx_agent_knowledges_agent_id
ON agent_knowledges (agent_id);

CREATE INDEX idx_agent_knowledges_knowledge_id
ON agent_knowledges (knowledge_id);

CREATE UNIQUE INDEX idx_agent_knowledge_unique
ON agent_knowledges (agent_id, knowledge_id)
WHERE deleted_at IS null;

CREATE TABLE agent_mcps (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    mcp_id UUID NOT NULL REFERENCES mcps (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_agent_mcps_agent_id ON agent_mcps (agent_id);
CREATE INDEX idx_agent_mcps_mcp_id ON agent_mcps (mcp_id);

CREATE UNIQUE INDEX idx_agent_mcp_unique
ON agent_mcps (agent_id, mcp_id)
WHERE deleted_at IS null;

CREATE VIEW agent_knowledges_view AS
SELECT
    id,
    agent_id,
    knowledge_id,
    is_active_prod,
    is_active_dev,
    created_at,
    updated_at,
    status
FROM agent_knowledges
WHERE deleted_at IS null;

CREATE VIEW agent_mcps_view AS
SELECT
    id,
    agent_id,
    mcp_id,
    created_at,
    updated_at
FROM agent_mcps
WHERE deleted_at IS null;

-- +goose Down
DROP VIEW IF EXISTS agent_mcps_view;
DROP VIEW IF EXISTS agent_knowledges_view;

DROP TABLE IF EXISTS agent_mcps;
DROP TABLE IF EXISTS agent_knowledges;
