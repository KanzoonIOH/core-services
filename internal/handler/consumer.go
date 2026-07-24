package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	flushSize     = 100
	flushEvery    = 5 * time.Second
	consumerGroup = "aic3-service"
)

// ChatMessageEvent is the shape of messages on chat.webhook.message.
type ChatMessageEvent struct {
	AgentID        string    `json:"agent_id"`
	ConversationID string    `json:"conversation_id"`
	StatusCode     int       `json:"status_code"`
	ResponseTimeMs int64     `json:"response_time_ms"`
	IsSuccess      bool      `json:"is_success"`
	Error          string    `json:"error"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// ChatMessageRow is what gets written to ClickHouse webhook_messages.
type ChatMessageRow struct {
	AgentID        string
	ConversationID string
	StatusCode     int32
	ResponseTimeMs int64
	IsSuccess      bool
	Error          *string
	OccurredAt     time.Time
}

// ConversationAnalyticsEvent is the shape of messages on chat.conversation.analytics.
// Published by the post-conversation NLP/enrichment worker once analysis is complete.
type ConversationAnalyticsEvent struct {
	ConversationID   string    `json:"conversation_id"`
	AgentID          string    `json:"agent_id"`
	SessionID        string    `json:"session_id"`
	StartedAt        time.Time `json:"started_at"`
	EndedAt          time.Time `json:"ended_at"`
	ResolutionMs     int64     `json:"resolution_ms"`
	EndReason        string    `json:"end_reason"`
	MessageCount     int32     `json:"message_count"`
	Channel          string    `json:"channel"`
	Intent           string    `json:"intent"`
	Intents          []string  `json:"intents"`
	Topics           []string  `json:"topics"`
	Sentiment        string    `json:"sentiment"`
	SentimentScore   float32   `json:"sentiment_score"`
	Language         string    `json:"language"`
	IsResolved       bool      `json:"is_resolved"`
	EscalationReason string    `json:"escalation_reason"`
	FirstResponseMs  *int64    `json:"first_response_ms,omitempty"`
	UserSatisfaction *int8     `json:"user_satisfaction,omitempty"`
	Summary          string    `json:"summary"`
	Keywords         []string  `json:"keywords"`
	ExternalUserID   string    `json:"external_user_id"`
	Tags             []string  `json:"tags"`
	ModelVersion     string    `json:"model_version"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// ConversationEndEvent is published when a conversation mechanically ends. For
// now only timed-out events are inserted directly, so dashboards can show
// timeout insight before the enrichment worker exists.
type ConversationEndEvent struct {
	ConversationID   string    `json:"conversation_id"`
	AgentID          string    `json:"agent_id"`
	SessionID        string    `json:"session_id"`
	StartedAt        time.Time `json:"started_at"`
	EndedAt          time.Time `json:"ended_at"`
	ResolutionMs     int64     `json:"resolution_ms"`
	EndReason        string    `json:"end_reason"`
	EscalationReason string    `json:"escalation_reason"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// ConversationAnalyticsRow is what gets written to ClickHouse conversation_analytics.
type ConversationAnalyticsRow struct {
	ConversationID   string
	AgentID          string
	SessionID        string
	StartedAt        time.Time
	EndedAt          time.Time
	ResolutionMs     int64
	EndReason        string
	MessageCount     int32
	Channel          string
	Intent           string
	Intents          []string
	Topics           []string
	Sentiment        string
	SentimentScore   float32
	Language         string
	IsResolved       bool
	EscalationReason *string
	FirstResponseMs  *int64
	UserSatisfaction *int8
	Summary          string
	Keywords         []string
	ExternalUserID   *string
	Tags             []string
	ModelVersion     string
	OccurredAt       time.Time
}

// StartChatConsumer runs a background goroutine that:
//  1. Consumes chat webhook/conversation events from Redpanda
//  2. Batches rows in memory per table
//  3. Flushes to ClickHouse every flushEvery or when flushSize is reached
//  4. On conversation.end, marks the Postgres conversation inactive
func StartChatConsumer(ctx context.Context, brokers []string, ch *lib.ClickHouseClient, conn *pgxpool.Pool) {
	queries := db.New(conn)

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(consumerGroup),
		kgo.ConsumeTopics(TopicChatMessage, TopicConversationAnalytics, TopicConversationEnd),
	)
	if err != nil {
		log.Fatalf("chat consumer: %v", err)
	}

	go func() {
		defer client.Close()

		var msgBatch []ChatMessageRow
		var analyticsBatch []ConversationAnalyticsRow

		ticker := time.NewTicker(flushEvery)
		defer ticker.Stop()

		flushPending := func() {
			flushCtx := ctx
			if ctx.Err() != nil {
				flushCtx = context.Background()
			}
			if len(msgBatch) > 0 {
				flushMessages(flushCtx, ch, msgBatch)
				msgBatch = msgBatch[:0]
			}
			if len(analyticsBatch) > 0 {
				flushAnalytics(flushCtx, ch, analyticsBatch)
				analyticsBatch = analyticsBatch[:0]
			}
		}

		pollEvery := 500 * time.Millisecond

		for {
			pollCtx, cancel := context.WithTimeout(ctx, pollEvery)
			fetches := client.PollRecords(pollCtx, flushSize)
			cancel()

			if fetches.IsClientClosed() {
				flushPending()
				return
			}

			fetches.EachError(func(_ string, _ int32, err error) {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				slog.ErrorContext(ctx, "chat consumer fetch error", "error", err)
			})

			fetches.EachRecord(func(rec *kgo.Record) {
				switch rec.Topic {
				case TopicChatMessage:
					var evt ChatMessageEvent
					if err := json.Unmarshal(rec.Value, &evt); err != nil {
						slog.ErrorContext(ctx, "chat consumer unmarshal message", "error", err)
						return
					}
					if _, err := uuid.Parse(evt.AgentID); err != nil {
						slog.WarnContext(ctx, "chat consumer skip message with invalid agent_id", "agent_id", evt.AgentID)
						return
					}
					occurredAt := evt.OccurredAt
					if occurredAt.IsZero() {
						occurredAt = time.Now().UTC()
					}
					row := ChatMessageRow{
						AgentID:        evt.AgentID,
						ConversationID: evt.ConversationID,
						StatusCode:     int32(evt.StatusCode),
						ResponseTimeMs: evt.ResponseTimeMs,
						IsSuccess:      evt.IsSuccess,
						OccurredAt:     occurredAt,
					}
					if evt.Error != "" {
						row.Error = &evt.Error
					}
					msgBatch = append(msgBatch, row)

				case TopicConversationAnalytics:
					var evt ConversationAnalyticsEvent
					if err := json.Unmarshal(rec.Value, &evt); err != nil {
						slog.ErrorContext(ctx, "chat consumer unmarshal analytics", "error", err)
						return
					}
					if _, err := uuid.Parse(evt.AgentID); err != nil {
						slog.WarnContext(ctx, "chat consumer skip analytics with invalid agent_id", "agent_id", evt.AgentID)
						return
					}
					occurredAt := evt.OccurredAt
					if occurredAt.IsZero() {
						occurredAt = evt.EndedAt
					}
					if occurredAt.IsZero() {
						occurredAt = time.Now().UTC()
					}
					row := ConversationAnalyticsRow{
						ConversationID: evt.ConversationID,
						AgentID:        evt.AgentID,
						SessionID:      evt.SessionID,
						StartedAt:      evt.StartedAt,
						EndedAt:        evt.EndedAt,
						ResolutionMs:   evt.ResolutionMs,
						EndReason:      evt.EndReason,
						MessageCount:   evt.MessageCount,
						Channel:        evt.Channel,
						Intent:         evt.Intent,
						Intents:        evt.Intents,
						Topics:         evt.Topics,
						Sentiment:      evt.Sentiment,
						SentimentScore: evt.SentimentScore,
						Language:       evt.Language,
						IsResolved:     evt.IsResolved,
						Summary:        evt.Summary,
						Keywords:       evt.Keywords,
						Tags:           evt.Tags,
						ModelVersion:   evt.ModelVersion,
						OccurredAt:     occurredAt,
					}
					if evt.EscalationReason != "" {
						row.EscalationReason = &evt.EscalationReason
					}
					if evt.ExternalUserID != "" {
						row.ExternalUserID = &evt.ExternalUserID
					}
					row.FirstResponseMs = evt.FirstResponseMs
					row.UserSatisfaction = evt.UserSatisfaction
					analyticsBatch = append(analyticsBatch, row)

				case TopicConversationEnd:
					var evt ConversationEndEvent
					if err := json.Unmarshal(rec.Value, &evt); err != nil {
						slog.ErrorContext(ctx, "chat consumer unmarshal conversation end", "error", err)
						return
					}
					slog.InfoContext(ctx, "chat consumer received conversation end", "conversation", evt.ConversationID, "reason", evt.EndReason)
					// Persist the end to Postgres for every reason (not just
					// timeouts): flip is_active=false and record end metadata.
					closeConversation(ctx, queries, evt)

					// ClickHouse analytics row is only inserted for timeouts for
					// now; other reasons are enriched later by the NLP worker via
					// chat.conversation.analytics.
					if evt.EndReason != EndReasonTimedOut {
						return
					}
					row, ok := timedOutAnalyticsRow(evt)
					if !ok {
						return
					}
					analyticsBatch = append(analyticsBatch, row)
				}
			})

			totalPending := len(msgBatch) + len(analyticsBatch)
			if totalPending >= flushSize {
				flushPending()
			}

			select {
			case <-ctx.Done():
				flushPending()
				return
			case <-ticker.C:
				flushPending()
			default:
			}
		}
	}()
}

// closeConversation marks the Postgres conversation inactive. The is_active
// guard in the query makes duplicate/replayed end events a no-op.
func closeConversation(ctx context.Context, queries db.Querier, evt ConversationEndEvent) {
	id, err := uuid.Parse(evt.ConversationID)
	if err != nil {
		slog.WarnContext(ctx, "chat consumer skip close with invalid conversation_id", "conversation_id", evt.ConversationID)
		return
	}
	if evt.EndReason == "" {
		slog.WarnContext(ctx, "chat consumer skip close with empty end_reason", "conversation_id", evt.ConversationID)
		return
	}

	endedAt := evt.EndedAt
	if endedAt.IsZero() {
		endedAt = evt.OccurredAt
	}
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}

	resolutionMs := evt.ResolutionMs
	if resolutionMs < 0 {
		resolutionMs = 0
	}

	// ctx may already be cancelled during shutdown; use a fresh ctx so the
	// close still lands.
	closeCtx := ctx
	if ctx.Err() != nil {
		closeCtx = context.Background()
	}

	if err := queries.CloseConversation(closeCtx, db.CloseConversationParams{
		ID:           id,
		EndedAt:      endedAt,
		EndReason:    evt.EndReason,
		ResolutionMs: resolutionMs,
	}); err != nil {
		slog.ErrorContext(ctx, "chat consumer close conversation", "conversation", evt.ConversationID, "error", err)
		return
	}
	slog.InfoContext(ctx, "postgres: closed conversation", "conversation", evt.ConversationID, "reason", evt.EndReason)
}

func timedOutAnalyticsRow(evt ConversationEndEvent) (ConversationAnalyticsRow, bool) {
	if _, err := uuid.Parse(evt.AgentID); err != nil {
		slog.Warn("chat consumer skip conversation end with invalid agent_id", "agent_id", evt.AgentID)
		return ConversationAnalyticsRow{}, false
	}
	if evt.ConversationID == "" {
		slog.Warn("chat consumer skip conversation end with empty conversation_id")
		return ConversationAnalyticsRow{}, false
	}

	endedAt := evt.EndedAt
	if endedAt.IsZero() {
		endedAt = evt.OccurredAt
	}
	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	}

	startedAt := evt.StartedAt
	if startedAt.IsZero() {
		startedAt = endedAt
	}

	resolutionMs := evt.ResolutionMs
	if resolutionMs <= 0 {
		resolutionMs = endedAt.Sub(startedAt).Milliseconds()
		if resolutionMs < 0 {
			resolutionMs = 0
		}
	}

	occurredAt := evt.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = endedAt
	}

	sessionID := evt.SessionID
	if sessionID == "" {
		sessionID = evt.ConversationID
	}

	var escalationReason *string
	if evt.EscalationReason != "" {
		escalationReason = &evt.EscalationReason
	}

	return ConversationAnalyticsRow{
		ConversationID:   evt.ConversationID,
		AgentID:          evt.AgentID,
		SessionID:        sessionID,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		ResolutionMs:     resolutionMs,
		EndReason:        EndReasonTimedOut,
		MessageCount:     0,
		Channel:          "webhook",
		Intent:           "",
		Intents:          []string{},
		Topics:           []string{},
		Sentiment:        "unknown",
		SentimentScore:   0,
		Language:         "",
		IsResolved:       false,
		EscalationReason: escalationReason,
		Summary:          "Conversation timed out",
		Keywords:         []string{},
		Tags:             []string{"timed_out"},
		ModelVersion:     "system-timeout",
		OccurredAt:       occurredAt,
	}, true
}

func flushMessages(ctx context.Context, ch *lib.ClickHouseClient, rows []ChatMessageRow) {
	b, err := ch.Conn().PrepareBatch(ctx,
		"INSERT INTO webhook_messages (agent_id, conversation_id, status_code, response_time_ms, is_success, error, occurred_at)",
	)
	if err != nil {
		slog.ErrorContext(ctx, "clickhouse prepare batch (webhook_messages)", "error", err)
		return
	}

	for _, r := range rows {
		if err := b.Append(r.AgentID, r.ConversationID, r.StatusCode, r.ResponseTimeMs, r.IsSuccess, r.Error, r.OccurredAt); err != nil {
			slog.ErrorContext(ctx, "clickhouse append webhook_messages", "error", err)
		}
	}

	if err := b.Send(); err != nil {
		slog.ErrorContext(ctx, "clickhouse batch send (webhook_messages)", "error", err)
		return
	}

	slog.InfoContext(ctx, "clickhouse: flushed webhook messages", "count", len(rows))
}

func flushAnalytics(ctx context.Context, ch *lib.ClickHouseClient, rows []ConversationAnalyticsRow) {
	b, err := ch.Conn().PrepareBatch(ctx,
		`INSERT INTO conversation_analytics (
			conversation_id, agent_id, session_id,
			started_at, ended_at, resolution_ms, end_reason, message_count, channel,
			intent, intents, topics,
			sentiment, sentiment_score, language,
			is_resolved, escalation_reason,
			first_response_ms, user_satisfaction,
			summary, keywords, external_user_id, tags, model_version,
			occurred_at
		)`,
	)
	if err != nil {
		slog.ErrorContext(ctx, "clickhouse prepare batch (conversation_analytics)", "error", err)
		return
	}

	for _, r := range rows {
		if err := b.Append(
			r.ConversationID, r.AgentID, r.SessionID,
			r.StartedAt, r.EndedAt, r.ResolutionMs, r.EndReason, r.MessageCount, r.Channel,
			r.Intent, r.Intents, r.Topics,
			r.Sentiment, r.SentimentScore, r.Language,
			r.IsResolved, r.EscalationReason,
			r.FirstResponseMs, r.UserSatisfaction,
			r.Summary, r.Keywords, r.ExternalUserID, r.Tags, r.ModelVersion,
			r.OccurredAt,
		); err != nil {
			slog.ErrorContext(ctx, "clickhouse append conversation_analytics", "error", err)
		}
	}

	if err := b.Send(); err != nil {
		slog.ErrorContext(ctx, "clickhouse batch send (conversation_analytics)", "error", err)
		return
	}

	slog.InfoContext(ctx, "clickhouse: flushed conversation analytics rows", "count", len(rows))
}
