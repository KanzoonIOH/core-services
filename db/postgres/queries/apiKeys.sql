-- name: InsertApiKey :one
INSERT INTO api_keys (name, token)
VALUES (
    sqlc.arg(name),
    sqlc.arg(token)
)
RETURNING id, name, token, created_at;

-- name: CountApiKeys :one
SELECT COUNT(*) FROM api_keys_view;

-- name: SelectApiKeyByToken :one
SELECT * FROM api_keys_view
WHERE token = sqlc.arg(token);

-- name: RevokeApiKey :execrows
UPDATE api_keys
SET revoked_at = now()
WHERE
    revoked_at IS NULL
    AND id = sqlc.arg(id);

-- name: SelectApiKeys :many
SELECT * FROM api_keys_view
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    created_at DESC
LIMIT coalesce(sqlc.narg('limit'), 10) OFFSET coalesce(sqlc.narg('offset'), 0);
