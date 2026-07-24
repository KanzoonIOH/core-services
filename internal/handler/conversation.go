package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EndReason values for the mechanical conversation close trigger.
// These mirror the PG CONVERSATION_END_REASON enum (lowercase on the wire).
// Note: "resolved" is NOT an end_reason — resolution is a separate quality
// outcome (is_resolved) derived later by the enrichment worker.
const (
	EndReasonEscalated        = "escalated"
	EndReasonTimedOut         = "timed_out"
	EndReasonHumanConfirmed   = "human_confirmed"
	EndReasonHumanIntercepted = "human_intercepted"
)

type ConversationHandler struct {
	Queries db.Querier
	Kafka   *lib.KafkaProducer
	// Redis client will be added here once the Redis integration is finalised.
	// Redis *redis.Client
}

func NewConversationHandler(conn *pgxpool.Pool, kafka *lib.KafkaProducer) *ConversationHandler {
	return &ConversationHandler{
		Queries: db.New(conn),
		Kafka:   kafka,
	}
}

// endConversationRequest is the JSON body for POST /api/chat/{id}/conversation/end.
type endConversationRequest struct {
	// SessionID is the conversation_id returned by ForwardChatWebhook via X-Session-Id.
	SessionID string `json:"session_id"`

	// EndReason must be one of: "escalated", "timed_out", "human_confirmed", "human_intercepted".
	EndReason string `json:"end_reason"`

	// EscalationReason is required when end_reason is "escalated".
	EscalationReason *string `json:"escalation_reason,omitempty"`

	// StartedAt is the timestamp of the first message in the conversation.
	// Derive this from Redis before calling this endpoint, or pass the value
	// that was returned when the session was created.
	// TODO: once Redis integration is wired, this can be fetched internally
	// from the Redis list instead of being supplied by the caller.
	StartedAt time.Time `json:"started_at"`
}

// EndConversation handles POST /api/chat/{id}/conversation/end.
//
// Flow (current stub):
//  1. Validate agent exists.
//  2. Parse and validate the request body.
//  3. TODO: read the Redis list for session_id to verify / enrich data.
//  4. TODO: delete the Redis key after reading.
//  5. Publish a chat.conversation.end event to Redpanda.
//  6. The enrichment worker consumes this, runs NLP analysis, and publishes
//     chat.conversation.analytics which the consumer flushes into ClickHouse.
func (h *ConversationHandler) EndConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	_, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	var req endConversationRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	if req.SessionID == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	switch req.EndReason {
	case EndReasonEscalated, EndReasonTimedOut, EndReasonHumanConfirmed, EndReasonHumanIntercepted:
		// valid
	default:
		lib.ResponseJSONError(w, http.StatusBadRequest, "end_reason must be one of: escalated, timed_out, human_confirmed, human_intercepted")
		return
	}

	if req.EndReason == EndReasonEscalated && (req.EscalationReason == nil || *req.EscalationReason == "") {
		lib.ResponseJSONError(w, http.StatusBadRequest, "escalation_reason is required when end_reason is escalated")
		return
	}

	if req.StartedAt.IsZero() {
		lib.ResponseJSONError(w, http.StatusBadRequest, "started_at is required")
		return
	}

	// TODO: read the Redis list for req.SessionID here.
	// Example (once Redis client is wired):
	//   messages, err := h.Redis.LRange(r.Context(), req.SessionID, 0, -1).Result()
	//   if err != nil { ... }
	//   // extract started_at from first message if not provided by caller
	//   h.Redis.Del(r.Context(), req.SessionID)

	endedAt := time.Now().UTC()
	resolutionMs := endedAt.Sub(req.StartedAt).Milliseconds()

	var escalationReason string
	if req.EscalationReason != nil {
		escalationReason = *req.EscalationReason
	}

	event := map[string]any{
		"agent_id":          id.String(),
		"conversation_id":   req.SessionID,
		"end_reason":        req.EndReason,
		"escalation_reason": escalationReason,
		"started_at":        req.StartedAt,
		"ended_at":          endedAt,
		"resolution_ms":     resolutionMs,
		"occurred_at":       endedAt,
	}

	if err := h.Kafka.Publish(context.Background(), TopicConversationEnd, event); err != nil {
		slog.ErrorContext(r.Context(), "conversation: kafka publish failed", "topic", TopicConversationEnd, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to publish conversation end event")
		return
	}
	slog.InfoContext(r.Context(), "conversation: kafka published", "topic", TopicConversationEnd, "conversation_id", req.SessionID, "end_reason", req.EndReason)

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"session_id":    req.SessionID,
		"end_reason":    req.EndReason,
		"resolution_ms": resolutionMs,
	}, nil)
}
