-- name: InsertApiKey :one
INSERT INTO api_keys (name, token, expires_at)
VALUES (
    sqlc.arg(name),
    sqlc.arg(token),
    sqlc.narg(expires_at)
)
RETURNING id, name, token, expires_at, created_at;

-- name: CountApiKeys :one
SELECT COUNT(*) FROM api_keys_view;

-- name: SelectApiKeyByToken :one
-- Auth lookup: rejects expired keys (NULL expires_at = never expires).
SELECT * FROM api_keys_view
WHERE token = sqlc.arg(token)
    AND (expires_at IS NULL OR expires_at > now());

-- name: RevokeApiKey :execrows
UPDATE api_keys
SET revoked_at = NOW()
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
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
