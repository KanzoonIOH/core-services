
-- name: InsertApiKey :one
INSERT INTO api_keys (name, token)
VALUES (
    sqlc.arg(name),
    sqlc.arg(token)
)
RETURNING *;

-- name: CountApiKeys :one
SELECT COUNT(*) FROM api_keys
WHERE revoked_at IS NULL;

-- name: SelectApiKeys :many
SELECT * FROM api_keys
WHERE revoked_at IS NULL
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    created_at DESC
LIMIT coalesce(sqlc.narg('limit'), 10) OFFSET coalesce(sqlc.narg('offset'), 0);
