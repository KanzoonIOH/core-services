package handler

import (
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// auditConsumerGroup is separate from the chat consumer group so the two
// pipelines track offsets independently.
const auditConsumerGroup = "aic3-audit"

// AuditLogRow is what gets written to ClickHouse audit_logs.
type AuditLogRow struct {
	OccurredAt time.Time
	UserID     string
	Role       string
	AuthMethod string
	Action     string
	Menu       string
	Method     string
	Path       string
	Status     int32
}

// StartAuditConsumer runs a background goroutine that consumes audit events
// from Redpanda, batches them, and flushes to ClickHouse. Mirrors
// StartChatConsumer but for a single topic/table.
func StartAuditConsumer(ctx context.Context, brokers []string, ch *lib.ClickHouseClient) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(auditConsumerGroup),
		kgo.ConsumeTopics(middleware.TopicAudit),
	)
	if err != nil {
		log.Fatalf("audit consumer: %v", err)
	}

	go func() {
		defer client.Close()

		var batch []AuditLogRow
		ticker := time.NewTicker(flushEvery)
		defer ticker.Stop()

		flushPending := func() {
			if len(batch) == 0 {
				return
			}
			flushCtx := ctx
			if ctx.Err() != nil {
				flushCtx = context.Background()
			}
			flushAuditLogs(flushCtx, ch, batch)
			batch = batch[:0]
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
				log.Printf("audit consumer fetch error: %v", err)
			})

			fetches.EachRecord(func(rec *kgo.Record) {
				var evt middleware.AuditEvent
				if err := json.Unmarshal(rec.Value, &evt); err != nil {
					log.Printf("audit consumer unmarshal: %v", err)
					return
				}
				occurredAt := evt.Time
				if occurredAt.IsZero() {
					occurredAt = time.Now().UTC()
				}
				batch = append(batch, AuditLogRow{
					OccurredAt: occurredAt,
					UserID:     evt.UserID,
					Role:       evt.Role,
					AuthMethod: evt.AuthMethod,
					Action:     evt.Action,
					Menu:       evt.Menu,
					Method:     evt.Method,
					Path:       evt.Path,
					Status:     int32(evt.Status),
				})
			})

			if len(batch) >= flushSize {
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

func flushAuditLogs(ctx context.Context, ch *lib.ClickHouseClient, rows []AuditLogRow) {
	b, err := ch.Conn().PrepareBatch(ctx,
		"INSERT INTO audit_logs (occurred_at, user_id, role, auth_method, action, menu, method, path, status)",
	)
	if err != nil {
		log.Printf("clickhouse prepare batch (audit_logs): %v", err)
		return
	}

	for _, r := range rows {
		if err := b.Append(
			r.OccurredAt, r.UserID, r.Role, r.AuthMethod,
			r.Action, r.Menu, r.Method, r.Path, r.Status,
		); err != nil {
			log.Printf("clickhouse append audit_logs: %v", err)
		}
	}

	if err := b.Send(); err != nil {
		log.Printf("clickhouse batch send (audit_logs): %v", err)
		return
	}

	log.Printf("clickhouse: flushed %d audit logs", len(rows))
}
