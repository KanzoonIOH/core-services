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
SELECT * FROM users_view
WHERE (
    sqlc.narg('role')::text IS NULL
    OR sqlc.narg('role')::text = ''
    OR role::text = sqlc.narg('role')::text
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    created_at DESC
LIMIT coalesce(sqlc.narg('limit'), 10) OFFSET coalesce(sqlc.narg('offset'), 0);

-- name: AcceptMember :one
UPDATE users
SET
    role = 'user',
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
    AND role = 'new'
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: UpdateMemberStatus :one
UPDATE users
SET
    role = sqlc.arg(role),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
    AND role != 'new'
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: SoftDeleteMember :execrows
UPDATE users
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);
