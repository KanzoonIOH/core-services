-- name: InsertUserRegister :one
INSERT INTO users (name, username, email, hashed_password)
VALUES (
    sqlc.arg(username),
    sqlc.arg(username),
    sqlc.arg(email),
    sqlc.arg(hashed_password)
)
RETURNING *;

-- name: SelectUserByLoginId :one
SELECT * FROM users
WHERE
    deleted_at IS null
    AND (
        username = sqlc.arg(login_id)
        OR email = sqlc.arg(login_id)
    )
LIMIT 1;
