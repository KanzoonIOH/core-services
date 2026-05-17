-- name: SelectUserById :one
SELECT * FROM users
WHERE
    deleted_at IS null
    AND
    id = sqlc.arg(id)
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
    deleted_at IS null
    AND
    id = sqlc.arg(id)
RETURNING *;
