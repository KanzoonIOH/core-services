package handler

import (
	"aiac-service/internal/lib"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	flushSize     = 100
	flushEvery    = 5 * time.Second
	consumerGroup = "aiac-service"
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

// ConversationEndEvent is the shape of messages on chat.conversation.end.
type ConversationEndEvent struct {
	AgentID          string    `json:"agent_id"`
	ConversationID   string    `json:"conversation_id"`
	EndReason        string    `json:"end_reason"`        // "resolved" | "escalated" | "timed_out"
	EscalationReason string    `json:"escalation_reason"` // non-empty only when escalated
	StartedAt        time.Time `json:"started_at"`
	EndedAt          time.Time `json:"ended_at"`
	ResolutionMs     int64     `json:"resolution_ms"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// ConversationEndRow is what gets written to ClickHouse conversation_events.
type ConversationEndRow struct {
	AgentID          string
	ConversationID   string
	EndReason        string
	EscalationReason *string
	StartedAt        time.Time
	EndedAt          time.Time
	ResolutionMs     int64
	OccurredAt       time.Time
}

// StartChatConsumer runs a background goroutine that:
//  1. Consumes chat.webhook.message and chat.conversation.end from Redpanda
//  2. Batches rows in memory per table
//  3. Flushes to ClickHouse every flushEvery or when flushSize is reached
func StartChatConsumer(ctx context.Context, brokers []string, ch *lib.ClickHouseClient) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(consumerGroup),
		kgo.ConsumeTopics(TopicChatMessage, TopicConversationEnd),
	)
	if err != nil {
		log.Fatalf("chat consumer: %v", err)
	}

	go func() {
		defer client.Close()

		var msgBatch []ChatMessageRow
		var convBatch []ConversationEndRow

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
			if len(convBatch) > 0 {
				flushConversations(flushCtx, ch, convBatch)
				convBatch = convBatch[:0]
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
				log.Printf("chat consumer fetch error: %v", err)
			})

			fetches.EachRecord(func(rec *kgo.Record) {
				switch rec.Topic {
				case TopicChatMessage:
					var evt ChatMessageEvent
					if err := json.Unmarshal(rec.Value, &evt); err != nil {
						log.Printf("chat consumer unmarshal message: %v", err)
						return
					}
					row := ChatMessageRow{
						AgentID:        evt.AgentID,
						ConversationID: evt.ConversationID,
						StatusCode:     int32(evt.StatusCode),
						ResponseTimeMs: evt.ResponseTimeMs,
						IsSuccess:      evt.IsSuccess,
						OccurredAt:     evt.OccurredAt,
					}
					if evt.Error != "" {
						row.Error = &evt.Error
					}
					msgBatch = append(msgBatch, row)

				case TopicConversationEnd:
					var evt ConversationEndEvent
					if err := json.Unmarshal(rec.Value, &evt); err != nil {
						log.Printf("chat consumer unmarshal conversation: %v", err)
						return
					}
					row := ConversationEndRow{
						AgentID:        evt.AgentID,
						ConversationID: evt.ConversationID,
						EndReason:      evt.EndReason,
						StartedAt:      evt.StartedAt,
						EndedAt:        evt.EndedAt,
						ResolutionMs:   evt.ResolutionMs,
						OccurredAt:     evt.OccurredAt,
					}
					if evt.EscalationReason != "" {
						row.EscalationReason = &evt.EscalationReason
					}
					convBatch = append(convBatch, row)
				}
			})

			totalPending := len(msgBatch) + len(convBatch)
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

func flushMessages(ctx context.Context, ch *lib.ClickHouseClient, rows []ChatMessageRow) {
	b, err := ch.Conn().PrepareBatch(ctx,
		"INSERT INTO webhook_messages (agent_id, conversation_id, status_code, response_time_ms, is_success, error, occurred_at)",
	)
	if err != nil {
		log.Printf("clickhouse prepare batch (webhook_messages): %v", err)
		return
	}

	for _, r := range rows {
		if err := b.Append(r.AgentID, r.ConversationID, r.StatusCode, r.ResponseTimeMs, r.IsSuccess, r.Error, r.OccurredAt); err != nil {
			log.Printf("clickhouse append webhook_messages: %v", err)
		}
	}

	if err := b.Send(); err != nil {
		log.Printf("clickhouse batch send (webhook_messages): %v", err)
		return
	}

	log.Printf("clickhouse: flushed %d webhook messages", len(rows))
}

func flushConversations(ctx context.Context, ch *lib.ClickHouseClient, rows []ConversationEndRow) {
	b, err := ch.Conn().PrepareBatch(ctx,
		"INSERT INTO conversation_events (agent_id, conversation_id, end_reason, escalation_reason, started_at, ended_at, resolution_ms, occurred_at)",
	)
	if err != nil {
		log.Printf("clickhouse prepare batch (conversation_events): %v", err)
		return
	}

	for _, r := range rows {
		var escalationReason *string
		if r.EscalationReason != nil {
			escalationReason = r.EscalationReason
		}
		if err := b.Append(r.AgentID, r.ConversationID, r.EndReason, escalationReason, r.StartedAt, r.EndedAt, r.ResolutionMs, r.OccurredAt); err != nil {
			log.Printf("clickhouse append conversation_events: %v", err)
		}
	}

	if err := b.Send(); err != nil {
		log.Printf("clickhouse batch send (conversation_events): %v", err)
		return
	}

	log.Printf("clickhouse: flushed %d conversation events", len(rows))
}
