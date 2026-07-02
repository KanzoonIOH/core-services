-- name: SelectUserById :one
SELECT * FROM users_view
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: SelectUserByIdWithPassword :one
SELECT
    id,
    name,
    username,
    email,
    role,
    hashed_password,
    created_at,
    updated_at
FROM users
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateUser :one
UPDATE users
SET
    name = coalesce(sqlc.narg(name), name),
    username = coalesce(sqlc.narg(username), username),
    email = coalesce(sqlc.narg(email), email),
    hashed_password = coalesce(sqlc.narg(hashed_password), hashed_password),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: CountMembers :one
SELECT count(*) FROM users_view
WHERE (
    sqlc.narg('role')::text IS NULL
    OR sqlc.narg('role')::text = ''
    OR role::text = sqlc.narg('role')::text
);

-- name: SelectMembers :many
-- invited_token: the active (non-revoked, non-expired) INVITE token, if any,
-- so the UI can surface the accept link for pending invites.
SELECT
    uv.*,
    (u.hashed_password IS NOT NULL)::bool AS has_password,
    COALESCE((
        SELECT uc.token
        FROM upcoming_changes uc
        WHERE uc.user_id = uv.id
            AND uc.type = 'INVITE'
            AND uc.revoked_at IS NULL
            AND uc.expired_at > now()
        ORDER BY uc.created_at DESC
        LIMIT 1
    ), '')::text AS invite_token
FROM users_view uv
JOIN users u ON u.id = uv.id
WHERE (
    sqlc.narg('role')::text IS NULL
    OR sqlc.narg('role')::text = ''
    OR uv.role::text = sqlc.narg('role')::text
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN uv.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN uv.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN uv.created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN uv.created_at END DESC,
    uv.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: InsertUserInvite :one
-- Creates a passwordless PENDING user for the email-invite flow. Username/name
-- default to the email until the invitee sets their own on accept.
INSERT INTO users (name, username, email, hashed_password)
VALUES (
    sqlc.arg(email),
    sqlc.arg(email),
    sqlc.arg(email),
    NULL
)
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: SetPasswordAndActivate :one
-- Invite accept: set the password and promote PENDING -> VIEWER in one step.
UPDATE users
SET
    hashed_password = sqlc.arg(hashed_password),
    role = 'VIEWER',
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: AcceptMember :one
UPDATE users
SET
    role = 'VIEWER',
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
    AND role = 'PENDING'
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: UpdateMemberStatus :one
UPDATE users
SET
    role = sqlc.arg(role),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
    AND role != 'PENDING'
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: SoftDeleteMember :execrows
UPDATE users
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);
