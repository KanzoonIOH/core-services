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
SELECT
    agents.*,
    (
        SELECT COUNT(*) FROM agent_knowledges ak
        WHERE ak.agent_id = agents.id AND ak.deleted_at IS NULL
    ) AS knowledges_count,
    (
        SELECT COUNT(*) FROM mcps m
        WHERE m.agent_id = agents.id AND m.deleted_at IS NULL
    ) AS mcps_count
FROM agents
WHERE
    deleted_at IS NULL
    AND agents.id = sqlc.arg(id)
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
RETURNING *;

-- name: DeleteAgent :execrows
DELETE FROM agents
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountAgents :one
SELECT COUNT(*) FROM agents;

-- name: SelectAgents :many
SELECT
    a.*,
    COALESCE(ak.knowledges_count, 0) AS knowledges_count,
    COALESCE(m.mcps_count, 0) AS mcps_count
FROM agents a
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*) AS knowledges_count
    FROM agent_knowledges
    WHERE deleted_at IS NULL
    GROUP BY agent_id
) ak ON ak.agent_id = a.id
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*) AS mcps_count
    FROM mcps
    WHERE deleted_at IS NULL
    GROUP BY agent_id
) m ON m.agent_id = a.id
WHERE
    a.deleted_at IS NULL
    AND (
        sqlc.narg('is_active')::bool IS NULL
        OR a.is_active = sqlc.narg('is_active')::bool
    )
ORDER BY
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_asc' THEN a.is_active
    END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_desc' THEN a.is_active
    END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN a.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN a.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN a.created_at END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN a.created_at
    END DESC,
    created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
