-- name: InsertMessage :exec
INSERT INTO messages (conversation_id, role, content, attachments, data)
VALUES (
    sqlc.arg(conversation_id),
    sqlc.arg(role),
    sqlc.narg(content),
    sqlc.arg(attachments),
    sqlc.narg(data)
);

-- name: ListMessagesByConversation :many
SELECT id, conversation_id, role, content, attachments, data, created_at
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id)
ORDER BY created_at;
