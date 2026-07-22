-- name: InsertUpcomingChange :one
INSERT INTO upcoming_changes (
    token,
    type,
    user_id,
    upcoming_value,
    expired_at
)
VALUES (
    sqlc.arg(token),
    sqlc.arg(type),
    sqlc.arg(user_id),
    sqlc.narg(upcoming_value),
    COALESCE(sqlc.narg(expired_at), now() + INTERVAL '1 hour')
)
RETURNING *;

-- name: SelectUpcomingChangeByToken :one
SELECT * FROM upcoming_changes
WHERE
    token = sqlc.arg(token)
    AND revoked_at IS NULL
    AND expired_at > now();

-- name: RevokeUpcomingChangeByID :exec
UPDATE upcoming_changes
SET revoked_at = now()
WHERE
    id = sqlc.arg(id)
    AND revoked_at IS NULL;
