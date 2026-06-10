package store

import (
	"context"
	"time"
)

type SummaryView struct {
	Total               int64   `json:"total"`
	SuccessCount        int64   `json:"success_count"`
	FailureCount        int64   `json:"failure_count"`
	SuccessRate         float64 `json:"success_rate"`
	P50ResponseMs       float64 `json:"p50_response_ms"`
	P95ResponseMs       float64 `json:"p95_response_ms"`
	P99ResponseMs       float64 `json:"p99_response_ms"`
	Success2xx          int64   `json:"success_2xx"`
	Redirect3xx         int64   `json:"redirect_3xx"`
	Client4xx           int64   `json:"client_4xx"`
	Server5xx           int64   `json:"server_5xx"`
	Gateway502          int64   `json:"gateway_502"`
	NoResponse          int64   `json:"no_response"`
	OtherStatus         int64   `json:"other_status"`
	UniqConversations   int64   `json:"uniq_conversations"`
	UniqAgents          int64   `json:"uniq_agents"`
	AvgMessagesPerConvo float64 `json:"avg_messages_per_convo"`
}

type TimeseriesPointView struct {
	Bucket        time.Time `json:"bucket"`
	Total         int64     `json:"total"`
	SuccessCount  int64     `json:"success_count"`
	SuccessRate   float64   `json:"success_rate"`
	P50ResponseMs float64   `json:"p50_response_ms"`
	P95ResponseMs float64   `json:"p95_response_ms"`
	P99ResponseMs float64   `json:"p99_response_ms"`
}

type AgentPerformanceRow struct {
	AgentID       string  `json:"agent_id"`
	Total         int64   `json:"total"`
	SuccessCount  int64   `json:"success_count"`
	SuccessRate   float64 `json:"success_rate"`
	P95ResponseMs float64 `json:"p95_response_ms"`
	Server5xx     int64   `json:"server_5xx"`
	NoResponse    int64   `json:"no_response"`
}

type TrafficHeatmapRow struct {
	DayOfWeek    uint8   `json:"day_of_week"`
	HourOfDay    uint8   `json:"hour_of_day"`
	Total        int64   `json:"total"`
	SuccessCount int64   `json:"success_count"`
	SuccessRate  float64 `json:"success_rate"`
}

const messagesSummary = `-- name: MessagesSummary :one
SELECT
    countMerge(total),
    countIfMerge(success_count),
    countIfMerge(failure_count),
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct),
    countIfMerge(success_2xx_count),
    countIfMerge(redirect_3xx_count),
    countIfMerge(client_4xx_count),
    countIfMerge(server_5xx_count),
    countIfMerge(gateway_502_count),
    countIfMerge(no_response_count),
    countIfMerge(other_status_count),
    uniqMerge(uniq_convos),
    uniqExact(agent_id)
FROM webhook_messages_hourly
WHERE bucket >= ?
`

const messagesSummaryByAgentId = `-- name: MessagesSummaryByAgentId :one
SELECT
    countMerge(total),
    countIfMerge(success_count),
    countIfMerge(failure_count),
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct),
    countIfMerge(success_2xx_count),
    countIfMerge(redirect_3xx_count),
    countIfMerge(client_4xx_count),
    countIfMerge(server_5xx_count),
    countIfMerge(gateway_502_count),
    countIfMerge(no_response_count),
    countIfMerge(other_status_count),
    uniqMerge(uniq_convos),
    uniqExact(agent_id)
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
		failureCount uint64
		percentiles  []float64
		success2xx   uint64
		redirect3xx  uint64
		client4xx    uint64
		server5xx    uint64
		gateway502   uint64
		noResponse   uint64
		otherStatus  uint64
		uniqConvos   uint64
		uniqAgents   uint64
	)
	if err := row.Scan(&total, &successCount, &failureCount, &percentiles, &success2xx, &redirect3xx, &client4xx, &server5xx, &gateway502, &noResponse, &otherStatus, &uniqConvos, &uniqAgents); err != nil {
		return SummaryView{}, err
	}

	p50, p95, p99 := splitPercentiles(percentiles)

	var successRate float64
	if total > 0 {
		successRate = float64(successCount) / float64(total)
	}

	var avgMessagesPerConvo float64
	if uniqConvos > 0 {
		avgMessagesPerConvo = float64(total) / float64(uniqConvos)
	}

	return SummaryView{
		Total:               int64(total),
		SuccessCount:        int64(successCount),
		FailureCount:        int64(failureCount),
		SuccessRate:         successRate,
		P50ResponseMs:       p50,
		P95ResponseMs:       p95,
		P99ResponseMs:       p99,
		Success2xx:          int64(success2xx),
		Redirect3xx:         int64(redirect3xx),
		Client4xx:           int64(client4xx),
		Server5xx:           int64(server5xx),
		Gateway502:          int64(gateway502),
		NoResponse:          int64(noResponse),
		OtherStatus:         int64(otherStatus),
		UniqConversations:   int64(uniqConvos),
		UniqAgents:          int64(uniqAgents),
		AvgMessagesPerConvo: avgMessagesPerConvo,
	}, nil
}

const messagesTimeseriesHourly = `-- name: MessagesTimeseriesHourly :many
SELECT
    toStartOfHour(bucket) AS step,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count,
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct) AS response_percentiles
FROM webhook_messages_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 HOUR
`

const messagesTimeseriesHourlyByAgentId = `-- name: MessagesTimeseriesHourlyByAgentId :many
SELECT
    toStartOfHour(bucket) AS step,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count,
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct) AS response_percentiles
FROM webhook_messages_hourly
WHERE bucket >= ? AND agent_id = ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 HOUR
`

const messagesTimeseriesDaily = `-- name: MessagesTimeseriesDaily :many
SELECT
    toStartOfDay(bucket) AS step,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count,
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct) AS response_percentiles
FROM webhook_messages_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 DAY
`

const messagesTimeseriesDailyByAgentId = `-- name: MessagesTimeseriesDailyByAgentId :many
SELECT
    toStartOfDay(bucket) AS step,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count,
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct) AS response_percentiles
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
			percentiles  []float64
		)
		if err := rows.Scan(&bucket, &total, &successCount, &percentiles); err != nil {
			return nil, err
		}

		p50, p95, p99 := splitPercentiles(percentiles)

		var rate float64
		if total > 0 {
			rate = float64(successCount) / float64(total)
		}

		items = append(items, TimeseriesPointView{
			Bucket:        bucket,
			Total:         int64(total),
			SuccessCount:  int64(successCount),
			SuccessRate:   rate,
			P50ResponseMs: p50,
			P95ResponseMs: p95,
			P99ResponseMs: p99,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const agentPerformance = `-- name: AgentPerformance :many
SELECT
    agent_id,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count,
    quantilesTDigestMerge(0.5, 0.95, 0.99)(resp_time_pct) AS response_percentiles,
    countIfMerge(server_5xx_count) AS server_5xx,
    countIfMerge(no_response_count) AS no_response
FROM webhook_messages_hourly
WHERE bucket >= ?
GROUP BY agent_id
HAVING total > 0
ORDER BY success_count / total ASC, arrayElement(response_percentiles, 2) DESC
LIMIT ?
`

type AgentPerformanceParams struct {
	Since time.Time `json:"since"`
	Limit int32     `json:"limit"`
}

func (q *Queries) AgentPerformance(ctx context.Context, arg AgentPerformanceParams) ([]AgentPerformanceRow, error) {
	rows, err := q.conn.Query(ctx, agentPerformance, arg.Since, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AgentPerformanceRow{}
	for rows.Next() {
		var (
			agentID      string
			total        uint64
			successCount uint64
			percentiles  []float64
			server5xx    uint64
			noResponse   uint64
		)
		if err := rows.Scan(&agentID, &total, &successCount, &percentiles, &server5xx, &noResponse); err != nil {
			return nil, err
		}

		_, p95, _ := splitPercentiles(percentiles)
		var rate float64
		if total > 0 {
			rate = float64(successCount) / float64(total)
		}

		items = append(items, AgentPerformanceRow{
			AgentID:       agentID,
			Total:         int64(total),
			SuccessCount:  int64(successCount),
			SuccessRate:   rate,
			P95ResponseMs: p95,
			Server5xx:     int64(server5xx),
			NoResponse:    int64(noResponse),
		})
	}
	return items, rows.Err()
}

const trafficHeatmap = `-- name: TrafficHeatmap :many
SELECT
    toDayOfWeek(bucket) AS day_of_week,
    toHour(bucket) AS hour_of_day,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count
FROM webhook_messages_hourly
WHERE bucket >= ?
GROUP BY day_of_week, hour_of_day
ORDER BY day_of_week, hour_of_day
`

const trafficHeatmapByAgentId = `-- name: TrafficHeatmapByAgentId :many
SELECT
    toDayOfWeek(bucket) AS day_of_week,
    toHour(bucket) AS hour_of_day,
    countMerge(total) AS total,
    countIfMerge(success_count) AS success_count
FROM webhook_messages_hourly
WHERE bucket >= ? AND agent_id = ?
GROUP BY day_of_week, hour_of_day
ORDER BY day_of_week, hour_of_day
`

type TrafficHeatmapParams struct {
	Since   time.Time `json:"since"`
	AgentID *string   `json:"agent_id"`
}

func (q *Queries) TrafficHeatmap(ctx context.Context, arg TrafficHeatmapParams) ([]TrafficHeatmapRow, error) {
	var query string
	var args []any
	if arg.AgentID != nil {
		query, args = trafficHeatmapByAgentId, []any{arg.Since, *arg.AgentID}
	} else {
		query, args = trafficHeatmap, []any{arg.Since}
	}

	rows, err := q.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TrafficHeatmapRow{}
	for rows.Next() {
		var (
			dayOfWeek    uint8
			hourOfDay    uint8
			total        uint64
			successCount uint64
		)
		if err := rows.Scan(&dayOfWeek, &hourOfDay, &total, &successCount); err != nil {
			return nil, err
		}

		var rate float64
		if total > 0 {
			rate = float64(successCount) / float64(total)
		}

		items = append(items, TrafficHeatmapRow{
			DayOfWeek:    dayOfWeek,
			HourOfDay:    hourOfDay,
			Total:        int64(total),
			SuccessCount: int64(successCount),
			SuccessRate:  rate,
		})
	}
	return items, rows.Err()
}

func splitPercentiles(percentiles []float64) (p50, p95, p99 float64) {
	if len(percentiles) > 0 {
		p50 = percentiles[0]
	}
	if len(percentiles) > 1 {
		p95 = percentiles[1]
	}
	if len(percentiles) > 2 {
		p99 = percentiles[2]
	}
	return p50, p95, p99
}

func truncateToDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
