package store

import "time"

type WebhookMessagesView struct {
	AgentID        string    `json:"agent_id"`
	ConversationID string    `json:"conversation_id"`
	StatusCode     int32     `json:"status_code"`
	ResponseTimeMs int64     `json:"response_time_ms"`
	IsSuccess      bool      `json:"is_success"`
	Error          *string   `json:"error"`
	OccurredAt     time.Time `json:"occurred_at"`
}
