-- name: InsertAgent :one
INSERT INTO agents (name, description, is_active, webhook_uri)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(is_active),
    sqlc.arg(webhook_uri)
)
RETURNING id, name, description, is_active, webhook_uri, tone, response_length, communication_style, created_at, updated_at;

-- name: SelectAgentById :one
SELECT
    av.*,
    (
        SELECT COUNT(*) FROM agent_knowledges_view akv
        WHERE akv.agent_id = av.id
    ) AS knowledges_count,
    (
        SELECT COUNT(*) FROM mcps_view mv
        WHERE mv.agent_id = av.id
    ) AS mcps_count
FROM agents_view av
WHERE av.id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateAgent :one
UPDATE agents
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    is_active = sqlc.arg(is_active),
    webhook_uri = sqlc.arg(webhook_uri),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, name, description, is_active, webhook_uri, tone, response_length, communication_style, created_at, updated_at;

-- name: UpdateAgentPersona :one
UPDATE agents
SET
    tone = sqlc.arg(tone),
    response_length = sqlc.arg(response_length),
    communication_style = sqlc.arg(communication_style),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, name, description, is_active, webhook_uri, tone, response_length, communication_style, created_at, updated_at;

-- name: SoftDeleteAgent :execrows
UPDATE agents
SET deleted_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountAgents :one
SELECT COUNT(*) FROM agents_view;

-- name: SelectAgents :many
SELECT
    av.*,
    COALESCE(ak.knowledges_count, 0) AS knowledges_count,
    COALESCE(m.mcps_count, 0) AS mcps_count
FROM agents_view av
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*) AS knowledges_count
    FROM agent_knowledges_view
    GROUP BY agent_id
) ak ON ak.agent_id = av.id
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*) AS mcps_count
    FROM mcps_view
    GROUP BY agent_id
) m ON m.agent_id = av.id
WHERE (
    sqlc.narg('is_active')::bool IS NULL
    OR av.is_active = sqlc.narg('is_active')::bool
)
ORDER BY
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_asc' THEN av.is_active
    END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_desc' THEN av.is_active
    END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN av.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN av.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN av.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN av.created_at
    END DESC,
    av.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
