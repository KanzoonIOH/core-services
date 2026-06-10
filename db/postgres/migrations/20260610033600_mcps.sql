-- +goose Up
CREATE TABLE mcps (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    description TEXT,
    uri TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE mcp_tools (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    description TEXT,
    target_uri TEXT NOT NULL,
    input_schema JSONB,
    mcp_id UUID NOT NULL REFERENCES mcps (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_mcp_tools_mcp_id ON mcp_tools (mcp_id);

CREATE VIEW mcps_view AS
SELECT
    id,
    name,
    description,
    uri,
    created_at,
    updated_at
FROM mcps
WHERE deleted_at IS NULL;

CREATE VIEW mcp_tools_view AS
SELECT
    id,
    mcp_id,
    name,
    description,
    target_uri,
    input_schema,
    created_at,
    updated_at
FROM mcp_tools
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS mcp_tools_view;
DROP VIEW IF EXISTS mcps_view;

DROP TABLE IF EXISTS mcp_tools;
DROP TABLE IF EXISTS mcps;
