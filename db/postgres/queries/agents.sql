-- name: InsertAgent :one
INSERT INTO agents (name, description, is_active, webhook_uri)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(is_active),
    sqlc.arg(webhook_uri)
)
RETURNING *;

-- name: SelectAgentById :one
SELECT * FROM agents
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateAgent :one
UPDATE agents
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    is_active = sqlc.arg(is_active),
    webhook_uri = sqlc.arg(webhook_uri),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteAgent :execrows
DELETE FROM agents
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: SelectAgents :many
SELECT * FROM agents
WHERE
    deleted_at IS NULL
    AND (
        sqlc.narg('is_active')::bool IS NULL
        OR is_active = sqlc.narg('is_active')::bool
    )
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'is_active_asc' THEN is_active END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'is_active_desc' THEN is_active END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- -- name: GetAgentKnowledges :many
-- SELECT k.* FROM knowledges k
-- JOIN agent_knowledges ak ON ak.knowledge_id = k.id
-- WHERE
--     k.deleted_at IS NULL
--     AND ak.deleted_at IS NULL
--     AND ak.agent_id = sqlc.arg(agent_id)
-- ORDER BY k.created_at DESC;

-- -- name: GetAgentMcps :many
-- SELECT m.* FROM mcps m
-- WHERE
--     m.deleted_at IS NULL
--     AND EXISTS (
--         SELECT 1 FROM agents a
--         WHERE a.deleted_at IS NULL AND a.id = sqlc.arg(agent_id)
--     )
--     AND FALSE
-- ORDER BY m.created_at DESC;
