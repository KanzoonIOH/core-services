-- name: InsertMcp :one
INSERT INTO mcps (agent_id, name, description, uri)
VALUES (
    sqlc.arg(agent_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(uri)
)
RETURNING id, agent_id, name, description, uri, created_at, updated_at;

-- name: SelectMcpById :one
SELECT * FROM mcps_view
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateMcp :one
UPDATE mcps
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, agent_id, name, description, uri, created_at, updated_at;

-- name: SoftDeleteMcp :execrows
UPDATE mcps
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountMcps :one
SELECT count(*) FROM mcps_view;

-- name: SelectMcps :many
SELECT
    m.*,
    (
        SELECT count(*)
        FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) AS tools_count
FROM mcps_view AS m
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN m.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN m.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN m.created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN m.created_at END DESC,
    m.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountMcpsByAgentId :one
SELECT count(*) FROM mcps_view
WHERE agent_id = sqlc.arg(agent_id);

-- name: SelectMcpsByAgentId :many
SELECT
    m.*,
    (
        SELECT count(*)
        FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) AS tools_count
FROM mcps_view AS m
WHERE m.agent_id = sqlc.arg(agent_id)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN m.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN m.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN m.created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN m.created_at END DESC,
    m.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
