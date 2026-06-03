package store

import "context"

const countMessages = `-- name: CountMessages :one
SELECT count(*) FROM webhook_messages
`

func (q *Queries) CountMessages(ctx context.Context) (int64, error) {
	row := q.conn.QueryRow(ctx, countMessages)
	var count uint64
	err := row.Scan(&count)
	return int64(count), err
}

const selectMessages = `-- name: SelectMessages :many
SELECT
    agent_id,
    conversation_id,
    status_code,
    response_time_ms,
    is_success,
    occurred_at
FROM webhook_messages
ORDER BY occurred_at DESC
LIMIT ?, ?
`

type SelectMessagesParams struct {
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func (q *Queries) SelectMessages(ctx context.Context, arg SelectMessagesParams) ([]WebhookMessagesView, error) {
	rows, err := q.conn.Query(ctx, selectMessages, arg.Offset, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []WebhookMessagesView{}
	for rows.Next() {
		var i WebhookMessagesView
		if err := rows.Scan(
			&i.AgentID,
			&i.ConversationID,
			&i.StatusCode,
			&i.ResponseTimeMs,
			&i.IsSuccess,
			&i.OccurredAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const countMessagesByAgentId = `-- name: CountMessagesByAgentId :one
SELECT count(*) FROM webhook_messages
WHERE agent_id = ?
`

func (q *Queries) CountMessagesByAgentId(ctx context.Context, agentID string) (int64, error) {
	row := q.conn.QueryRow(ctx, countMessagesByAgentId, agentID)
	var count uint64
	err := row.Scan(&count)
	return int64(count), err
}

const selectMessagesByAgentId = `-- name: SelectMessagesByAgentId :many
SELECT
    agent_id,
    conversation_id,
    status_code,
    response_time_ms,
    is_success,
    occurred_at
FROM webhook_messages
WHERE agent_id = ?
ORDER BY occurred_at DESC
LIMIT ?, ?
`

type SelectMessagesByAgentIdParams struct {
	AgentID string `json:"agent_id"`
	Limit   int32  `json:"limit"`
	Offset  int32  `json:"offset"`
}

func (q *Queries) SelectMessagesByAgentId(ctx context.Context, arg SelectMessagesByAgentIdParams) ([]WebhookMessagesView, error) {
	rows, err := q.conn.Query(ctx, selectMessagesByAgentId, arg.AgentID, arg.Offset, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []WebhookMessagesView{}
	for rows.Next() {
		var i WebhookMessagesView
		if err := rows.Scan(
			&i.AgentID,
			&i.ConversationID,
			&i.StatusCode,
			&i.ResponseTimeMs,
			&i.IsSuccess,
			&i.OccurredAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
