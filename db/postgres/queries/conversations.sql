-- name: UpsertConversation :exec
INSERT INTO conversations (id, agent_id)
VALUES (sqlc.arg(id), sqlc.arg(agent_id))
ON CONFLICT (id) DO NOTHING;

-- name: CountConversations :one
SELECT count(*) FROM conversations_view;

-- name: SelectConversations :many
SELECT
    c.id,
    c.agent_id,
    a.name AS agent_name,
    c.started_at,
    c.ended_at,
    c.is_active,
    lm.content AS last_message,
    lm.created_at AS last_message_at,
    (
        SELECT count(*) FROM messages AS m
        WHERE m.conversation_id = c.id
    ) AS message_count
FROM conversations_view AS c
INNER JOIN agents AS a ON c.agent_id = a.id
LEFT JOIN LATERAL (
    SELECT
        m.content,
        m.created_at
    FROM messages AS m
    WHERE m.conversation_id = c.id
    ORDER BY m.created_at DESC
    LIMIT 1
) AS lm ON true
ORDER BY coalesce(lm.created_at, c.started_at) DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CloseConversation :exec
-- Marks a conversation ended. end_reason arrives lowercase on the wire
-- (e.g. "timed_out"); upper() maps it onto the CONVERSATION_END_REASON enum.
-- Guarded by is_active so a replayed/duplicate end event is a no-op.
UPDATE conversations
SET
    is_active = false,
    ended_at = sqlc.arg(ended_at)::TIMESTAMPTZ,
    end_reason = upper(sqlc.arg(end_reason)::TEXT)::CONVERSATION_END_REASON,
    resolution_ms = sqlc.arg(resolution_ms)::BIGINT
WHERE id = sqlc.arg(id) AND is_active = true;

-- name: DeleteConversation :exec
DELETE FROM conversations WHERE id = sqlc.arg(id);

-- name: DeleteMessagesByConversation :exec
DELETE FROM messages WHERE conversation_id = sqlc.arg(conversation_id);

-- name: SelectConversationById :one
SELECT
    c.id,
    c.agent_id,
    a.name AS agent_name,
    c.started_at,
    c.ended_at,
    c.is_active
FROM conversations_view AS c
INNER JOIN agents AS a ON c.agent_id = a.id
WHERE c.id = sqlc.arg(id);
