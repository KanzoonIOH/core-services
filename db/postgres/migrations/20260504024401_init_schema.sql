-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE USER_ROLE AS ENUM (
    'admin',
    'user',
    'new'
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    role USER_ROLE NOT NULL DEFAULT 'new',
    hashed_password TEXT NOT NULL,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_users_email_unique ON users (
    lower(trim(email))
) WHERE deleted_at IS null;
CREATE UNIQUE INDEX idx_users_username_unique ON users (
    lower(trim(username))
) WHERE deleted_at IS null;

CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT false,
    webhook_uri TEXT NOT NULL,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE knowledges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    source_type TEXT NOT NULL,
    source_uri TEXT,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE agent_knowledges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    is_active_prod BOOLEAN NOT NULL DEFAULT false,
    is_active_dev BOOLEAN NOT NULL DEFAULT false,
    --
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    knowledge_id UUID NOT NULL REFERENCES knowledges (id) ON DELETE CASCADE,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_agent_knowledges_agent_id ON agent_knowledges (agent_id);
CREATE INDEX idx_agent_knowledges_knowledge_id ON agent_knowledges (
    knowledge_id
);
CREATE UNIQUE INDEX idx_agent_knowledge_unique
ON agent_knowledges (agent_id, knowledge_id)
WHERE deleted_at IS null;

CREATE TABLE mcps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    uri TEXT NOT NULL,
    --
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_mcps_agent_id ON mcps (agent_id);

CREATE TABLE mcp_tools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    target_uri TEXT NOT NULL,
    --
    mcp_id UUID NOT NULL REFERENCES mcps (id) ON DELETE CASCADE,
    --
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_mcp_functions_mcp_id ON mcp_tools (mcp_id);

-- +goose Down
DROP TABLE IF EXISTS mcp_functions;
DROP TABLE IF EXISTS mcps;
DROP TABLE IF EXISTS agent_knowledges;
DROP TABLE IF EXISTS knowledges;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS USER_ROLE;
DROP EXTENSION IF EXISTS pgcrypto;
