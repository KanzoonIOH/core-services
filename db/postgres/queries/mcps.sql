-- name: InsertMcp :one
INSERT INTO mcps (agent_id, name, description, uri)
VALUES (
    sqlc.arg(agent_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(uri)
)
RETURNING *;

-- name: SelectMcpById :one
SELECT * FROM mcps
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
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
RETURNING *;

-- name: DeleteMcp :execrows
DELETE FROM mcps
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: SelectMcps :many
SELECT * FROM mcps
WHERE deleted_at IS NULL
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    created_at DESC
LIMIT coalesce(sqlc.narg('limit'), 10) OFFSET coalesce(sqlc.narg('offset'), 0);
