package store

import (
	"context"
	"time"
)

type SummaryView struct {
	Total             int64   `json:"total"`
	SuccessCount      int64   `json:"success_count"`
	SuccessRate       float64 `json:"success_rate"`
	AvgResponseMs     float64 `json:"avg_response_ms"`
	UniqConversations int64   `json:"uniq_conversations"`
}

type TimeseriesPointView struct {
	Bucket        time.Time `json:"bucket"`
	Total         int64     `json:"total"`
	SuccessCount  int64     `json:"success_count"`
	SuccessRate   float64   `json:"success_rate"`
	AvgResponseMs float64   `json:"avg_response_ms"`
}

const messagesSummary = `-- name: MessagesSummary :one
SELECT
    countMerge(total),
    countIfMerge(success_count),
    avgMerge(resp_time_avg),
    uniqMerge(uniq_convos)
FROM webhook_messages_hourly
WHERE bucket >= ?
`

const messagesSummaryByAgentId = `-- name: MessagesSummaryByAgentId :one
SELECT
    countMerge(total),
    countIfMerge(success_count),
    avgMerge(resp_time_avg),
    uniqMerge(uniq_convos)
FROM webhook_messages_hourly
WHERE bucket >= ? AND agent_id = ?
`

// MessagesSummaryParams scopes a summary read. AgentID nil => global (all agents).
type MessagesSummaryParams struct {
	Since   time.Time `json:"since"`
	AgentID *string   `json:"agent_id"`
}

func (q *Queries) MessagesSummary(ctx context.Context, arg MessagesSummaryParams) (SummaryView, error) {
	var row interface {
		Scan(dest ...any) error
	}
	if arg.AgentID != nil {
		row = q.conn.QueryRow(ctx, messagesSummaryByAgentId, arg.Since, *arg.AgentID)
	} else {
		row = q.conn.QueryRow(ctx, messagesSummary, arg.Since)
	}

	var (
		total        uint64
		successCount uint64
		avgResponse  float64
		uniqConvos   uint64
	)
	if err := row.Scan(&total, &successCount, &avgResponse, &uniqConvos); err != nil {
		return SummaryView{}, err
	}

	var rate float64
	if total > 0 {
		rate = float64(successCount) / float64(total)
	}

	return SummaryView{
		Total:             int64(total),
		SuccessCount:      int64(successCount),
		SuccessRate:       rate,
		AvgResponseMs:     avgResponse,
		UniqConversations: int64(uniqConvos),
	}, nil
}

const messagesTimeseriesHourly = `-- name: MessagesTimeseriesHourly :many
SELECT toStartOfHour(bucket) AS step, countMerge(total) AS total,
    countIfMerge(success_count) AS success_count, avgMerge(resp_time_avg) AS avg_response_ms
FROM webhook_messages_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 HOUR
`

const messagesTimeseriesHourlyByAgentId = `-- name: MessagesTimeseriesHourlyByAgentId :many
SELECT toStartOfHour(bucket) AS step, countMerge(total) AS total,
    countIfMerge(success_count) AS success_count, avgMerge(resp_time_avg) AS avg_response_ms
FROM webhook_messages_hourly
WHERE bucket >= ? AND agent_id = ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 HOUR
`

const messagesTimeseriesDaily = `-- name: MessagesTimeseriesDaily :many
SELECT toStartOfDay(bucket) AS step, countMerge(total) AS total,
    countIfMerge(success_count) AS success_count, avgMerge(resp_time_avg) AS avg_response_ms
FROM webhook_messages_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 DAY
`

const messagesTimeseriesDailyByAgentId = `-- name: MessagesTimeseriesDailyByAgentId :many
SELECT toStartOfDay(bucket) AS step, countMerge(total) AS total,
    countIfMerge(success_count) AS success_count, avgMerge(resp_time_avg) AS avg_response_ms
FROM webhook_messages_hourly
WHERE bucket >= ? AND agent_id = ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 DAY
`

type MessagesTimeseriesParams struct {
	Since   time.Time `json:"since"`
	AgentID *string   `json:"agent_id"`
	Step    string    `json:"step"`
}

func (q *Queries) MessagesTimeseries(ctx context.Context, arg MessagesTimeseriesParams) ([]TimeseriesPointView, error) {
	var fillFrom, fillTo time.Time
	now := time.Now().UTC()
	if arg.Step == "hour" {
		fillFrom = arg.Since.Truncate(time.Hour)
		fillTo = now.Truncate(time.Hour).Add(time.Hour)
	} else {
		fillFrom = truncateToDay(arg.Since)
		fillTo = truncateToDay(now).AddDate(0, 0, 1)
	}

	var (
		query string
		args  []any
	)
	switch {
	case arg.Step == "hour" && arg.AgentID != nil:
		query, args = messagesTimeseriesHourlyByAgentId, []any{arg.Since, *arg.AgentID, fillFrom, fillTo}
	case arg.Step == "hour":
		query, args = messagesTimeseriesHourly, []any{arg.Since, fillFrom, fillTo}
	case arg.AgentID != nil:
		query, args = messagesTimeseriesDailyByAgentId, []any{arg.Since, *arg.AgentID, fillFrom, fillTo}
	default:
		query, args = messagesTimeseriesDaily, []any{arg.Since, fillFrom, fillTo}
	}

	rows, err := q.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TimeseriesPointView{}
	for rows.Next() {
		var (
			bucket       time.Time
			total        uint64
			successCount uint64
			avgResponse  float64
		)
		if err := rows.Scan(&bucket, &total, &successCount, &avgResponse); err != nil {
			return nil, err
		}

		var rate float64
		if total > 0 {
			rate = float64(successCount) / float64(total)
		}

		items = append(items, TimeseriesPointView{
			Bucket:        bucket,
			Total:         int64(total),
			SuccessCount:  int64(successCount),
			SuccessRate:   rate,
			AvgResponseMs: avgResponse,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func truncateToDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
