-- name: InsertUserRegister :one
INSERT INTO users (name, username, email, hashed_password)
VALUES (
    sqlc.arg(username),
    sqlc.arg(username),
    sqlc.arg(email),
    sqlc.arg(hashed_password)
)
RETURNING id, name, username, email, role, created_at, updated_at;

-- name: SelectUserByLoginIdWithPassword :one
SELECT id, name, username, email, role, hashed_password, created_at, updated_at
FROM users
WHERE
    deleted_at IS NULL
    AND (
        username = sqlc.arg(login_id)
        OR email = sqlc.arg(login_id)
    )
LIMIT 1;
