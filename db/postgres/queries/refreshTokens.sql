-- name: InsertRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip, expires_at)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(token_hash),
    sqlc.arg(user_agent),
    sqlc.arg(ip),
    sqlc.arg(expires_at)
)
RETURNING id, user_id, expires_at, created_at;

-- name: SelectActiveRefreshToken :one
-- Returns the session together with the CURRENT user role/deleted state, so the
-- refresh handler re-checks the live user on every refresh (a suspended or
-- soft-deleted user's session stops refreshing immediately).
SELECT
    rt.id,
    rt.user_id,
    rt.expires_at,
    u.role,
    u.deleted_at
FROM refresh_tokens rt
JOIN users u ON u.id = rt.user_id
WHERE rt.token_hash = sqlc.arg(token_hash)
    AND rt.revoked_at IS NULL
    AND rt.expires_at > now()
LIMIT 1;

-- name: TouchRefreshToken :exec
UPDATE refresh_tokens
SET last_used_at = now()
WHERE id = sqlc.arg(id);

-- name: RevokeRefreshTokenByHash :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: RevokeRefreshTokenByID :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshTokens :exec
-- Kill every active session for a user (logout-all / on status change).
UPDATE refresh_tokens
SET revoked_at = now()
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;
