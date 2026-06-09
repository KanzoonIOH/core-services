-- +goose Up
CREATE TABLE agent_mcps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    --
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    mcp_id UUID NOT NULL REFERENCES mcps (id) ON DELETE CASCADE,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_agent_mcps_agent_id ON agent_mcps (agent_id);
CREATE INDEX idx_agent_mcps_mcp_id ON agent_mcps (mcp_id);
CREATE UNIQUE INDEX idx_agent_mcp_unique
ON agent_mcps (agent_id, mcp_id)
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

DROP VIEW mcps_view;

ALTER TABLE mcps DROP COLUMN agent_id;

CREATE VIEW mcps_view AS
SELECT
    id,
    name,
    description,
    uri,
    created_at,
    updated_at
FROM mcps
WHERE deleted_at IS null;

-- +goose Down
DROP VIEW mcps_view;

ALTER TABLE mcps
ADD COLUMN agent_id UUID REFERENCES agents (id) ON DELETE CASCADE;

CREATE INDEX idx_mcps_agent_id ON mcps (agent_id);

CREATE VIEW mcps_view AS
SELECT
    id,
    agent_id,
    name,
    description,
    uri,
    created_at,
    updated_at
FROM mcps
WHERE deleted_at IS null;

DROP VIEW IF EXISTS agent_mcps_view;
DROP TABLE IF EXISTS agent_mcps;
