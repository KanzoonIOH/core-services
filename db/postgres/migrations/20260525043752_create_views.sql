-- +goose Up
CREATE VIEW users_view AS
SELECT
    id,
    name,
    username,
    email,
    role,
    created_at,
    updated_at
FROM users
WHERE deleted_at IS NULL;

CREATE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    is_active,
    webhook_uri,
    created_at,
    updated_at
FROM agents
WHERE deleted_at IS NULL;

CREATE VIEW knowledges_view AS
SELECT
    id,
    name,
    description,
    source_type,
    source_uri,
    created_at,
    updated_at
FROM knowledges
WHERE deleted_at IS NULL;

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
WHERE deleted_at IS NULL;

CREATE VIEW agent_knowledges_view AS
SELECT
    id,
    agent_id,
    knowledge_id,
    is_active_prod,
    is_active_dev,
    created_at,
    updated_at
FROM agent_knowledges
WHERE deleted_at IS NULL;

CREATE VIEW api_keys_view AS
SELECT
    id,
    name,
    token,
    created_at
FROM api_keys
WHERE revoked_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS api_keys_view;
DROP VIEW IF EXISTS agent_knowledges_view;
DROP VIEW IF EXISTS mcps_view;
DROP VIEW IF EXISTS knowledges_view;
DROP VIEW IF EXISTS agents_view;
DROP VIEW IF EXISTS users_view;
