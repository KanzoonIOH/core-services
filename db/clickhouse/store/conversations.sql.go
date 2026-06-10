package store

import (
	"context"
	"time"
)

type ConversationSummaryView struct {
	Total                      int64   `json:"total"`
	ResolvedCount              int64   `json:"resolved_count"`
	EscalatedCount             int64   `json:"escalated_count"`
	TimedOutCount              int64   `json:"timed_out_count"`
	InterceptedCount           int64   `json:"intercepted_count"`
	ResolutionRate             float64 `json:"resolution_rate"`
	EscalationRate             float64 `json:"escalation_rate"`
	AvgResolutionMs            float64 `json:"avg_resolution_ms"`
	AvgFirstResponseMs         float64 `json:"avg_first_response_ms"`
	AvgUserSatisfaction        float64 `json:"avg_user_satisfaction"`
	AvgMessagesPerConversation float64 `json:"avg_messages_per_conversation"`
	UniqUsers                  int64   `json:"uniq_users"`
	PositiveCount              int64   `json:"positive_count"`
	NeutralCount               int64   `json:"neutral_count"`
	NegativeCount              int64   `json:"negative_count"`
	OtherSentimentCount        int64   `json:"other_sentiment_count"`
}

type ConversationTimeseriesPoint struct {
	Bucket          time.Time `json:"bucket"`
	Total           int64     `json:"total"`
	ResolvedCount   int64     `json:"resolved_count"`
	EscalatedCount  int64     `json:"escalated_count"`
	ResolutionRate  float64   `json:"resolution_rate"`
	AvgResolutionMs float64   `json:"avg_resolution_ms"`
}

type IntentStatRow struct {
	Intent         string  `json:"intent"`
	Total          int64   `json:"total"`
	ResolvedCount  int64   `json:"resolved_count"`
	ResolutionRate float64 `json:"resolution_rate"`
}

type TopicStatRow struct {
	Topic          string  `json:"topic"`
	Total          int64   `json:"total"`
	ResolvedCount  int64   `json:"resolved_count"`
	ResolutionRate float64 `json:"resolution_rate"`
}

type SentimentStatRow struct {
	Sentiment string  `json:"sentiment"`
	Total     int64   `json:"total"`
	Pct       float64 `json:"pct"`
}

type ConversationsSummaryParams struct {
	AgentID *string
	Since   time.Time
}

type ConversationsTimeseriesParams struct {
	AgentID *string
	Since   time.Time
	Step    string // "hour" | "day" | "week"
}

type IntentStatsParams struct {
	AgentID *string
	Since   time.Time
	Limit   int32
}

type TopicStatsParams struct {
	AgentID *string
	Since   time.Time
	Limit   int32
}

type SentimentStatsParams struct {
	AgentID *string
	Since   time.Time
}

const conversationsSummary = `-- name: ConversationsSummary :one
SELECT
    count() AS total,
    countIf(is_resolved) AS resolved_count,
    countIf(end_reason = 'escalated') AS escalated_count,
    countIf(end_reason = 'timed_out') AS timed_out_count,
    countIf(end_reason = 'human_intercepted') AS intercepted_count,
    sum(resolution_ms) AS resolution_ms_sum,
    uniqExactIf(assumeNotNull(external_user_id), isNotNull(external_user_id)) AS uniq_users,
    countIf(sentiment = 'positive') AS positive_count,
    countIf(sentiment = 'neutral') AS neutral_count,
    countIf(sentiment = 'negative') AS negative_count,
    countIf(sentiment NOT IN ('positive', 'neutral', 'negative')) AS other_sentiment_count,
    sum(message_count) AS message_count_sum,
    sumIf(assumeNotNull(first_response_ms), isNotNull(first_response_ms)) AS first_response_ms_sum,
    countIf(isNotNull(first_response_ms)) AS first_response_count,
    sumIf(assumeNotNull(user_satisfaction), isNotNull(user_satisfaction)) AS user_satisfaction_sum,
    countIf(isNotNull(user_satisfaction)) AS user_satisfaction_count
FROM conversation_analytics FINAL
WHERE occurred_at >= ?
`

const conversationsSummaryByAgentId = `-- name: ConversationsSummaryByAgentId :one
SELECT
    count() AS total,
    countIf(is_resolved) AS resolved_count,
    countIf(end_reason = 'escalated') AS escalated_count,
    countIf(end_reason = 'timed_out') AS timed_out_count,
    countIf(end_reason = 'human_intercepted') AS intercepted_count,
    sum(resolution_ms) AS resolution_ms_sum,
    uniqExactIf(assumeNotNull(external_user_id), isNotNull(external_user_id)) AS uniq_users,
    countIf(sentiment = 'positive') AS positive_count,
    countIf(sentiment = 'neutral') AS neutral_count,
    countIf(sentiment = 'negative') AS negative_count,
    countIf(sentiment NOT IN ('positive', 'neutral', 'negative')) AS other_sentiment_count,
    sum(message_count) AS message_count_sum,
    sumIf(assumeNotNull(first_response_ms), isNotNull(first_response_ms)) AS first_response_ms_sum,
    countIf(isNotNull(first_response_ms)) AS first_response_count,
    sumIf(assumeNotNull(user_satisfaction), isNotNull(user_satisfaction)) AS user_satisfaction_sum,
    countIf(isNotNull(user_satisfaction)) AS user_satisfaction_count
FROM conversation_analytics FINAL
WHERE occurred_at >= ?
AND agent_id = ?
`

const conversationsTimeseriesHourly = `-- name: ConversationsTimeseriesHourly :many
SELECT
    toStartOfHour(bucket) AS step,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count,
    sum(escalated_count) AS escalated_count,
    if(sum(total) = 0, 0, toFloat64(sum(resolution_ms_sum)) / sum(total)) AS avg_resolution_ms
FROM conversation_analytics_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 HOUR
`

const conversationsTimeseriesHourlyByAgentId = `-- name: ConversationsTimeseriesHourlyByAgentId :many
SELECT
    toStartOfHour(bucket) AS step,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count,
    sum(escalated_count) AS escalated_count,
    if(sum(total) = 0, 0, toFloat64(sum(resolution_ms_sum)) / sum(total)) AS avg_resolution_ms
FROM conversation_analytics_hourly
WHERE bucket >= ?
AND agent_id = ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 HOUR
`

const conversationsTimeseriesDaily = `-- name: ConversationsTimeseriesDaily :many
SELECT
    toStartOfDay(bucket) AS step,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count,
    sum(escalated_count) AS escalated_count,
    if(sum(total) = 0, 0, toFloat64(sum(resolution_ms_sum)) / sum(total)) AS avg_resolution_ms
FROM conversation_analytics_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 DAY
`

const conversationsTimeseriesDailyByAgentId = `-- name: ConversationsTimeseriesDailyByAgentId :many
SELECT
    toStartOfDay(bucket) AS step,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count,
    sum(escalated_count) AS escalated_count,
    if(sum(total) = 0, 0, toFloat64(sum(resolution_ms_sum)) / sum(total)) AS avg_resolution_ms
FROM conversation_analytics_hourly
WHERE bucket >= ?
AND agent_id = ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 DAY
`

const conversationsTimeseriesWeekly = `-- name: ConversationsTimeseriesWeekly :many
SELECT
    toStartOfWeek(bucket, 1) AS step,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count,
    sum(escalated_count) AS escalated_count,
    if(sum(total) = 0, 0, toFloat64(sum(resolution_ms_sum)) / sum(total)) AS avg_resolution_ms
FROM conversation_analytics_hourly
WHERE bucket >= ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 WEEK
`

const conversationsTimeseriesWeeklyByAgentId = `-- name: ConversationsTimeseriesWeeklyByAgentId :many
SELECT
    toStartOfWeek(bucket, 1) AS step,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count,
    sum(escalated_count) AS escalated_count,
    if(sum(total) = 0, 0, toFloat64(sum(resolution_ms_sum)) / sum(total)) AS avg_resolution_ms
FROM conversation_analytics_hourly
WHERE bucket >= ?
AND agent_id = ?
GROUP BY step
ORDER BY step WITH FILL FROM ? TO ? STEP INTERVAL 1 WEEK
`

const intentStats = `-- name: IntentStats :many
SELECT
    intent,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count
FROM intent_stats
WHERE day >= toDate(?)
GROUP BY intent
ORDER BY total DESC
LIMIT ?
`

const intentStatsByAgentId = `-- name: IntentStatsByAgentId :many
SELECT
    intent,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count
FROM intent_stats
WHERE day >= toDate(?)
AND agent_id = ?
GROUP BY intent
ORDER BY total DESC
LIMIT ?
`

const topicStats = `-- name: TopicStats :many
SELECT
    topic,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count
FROM topic_stats
WHERE day >= toDate(?)
GROUP BY topic
ORDER BY total DESC
LIMIT ?
`

const topicStatsByAgentId = `-- name: TopicStatsByAgentId :many
SELECT
    topic,
    sum(total) AS total,
    sum(resolved_count) AS resolved_count
FROM topic_stats
WHERE day >= toDate(?)
AND agent_id = ?
GROUP BY topic
ORDER BY total DESC
LIMIT ?
`

const sentimentStats = `-- name: SentimentStats :one
SELECT
    sum(positive_count) AS positive,
    sum(neutral_count) AS neutral,
    sum(negative_count) AS negative,
    sum(total) AS total
FROM conversation_analytics_hourly
WHERE bucket >= ?
`

const sentimentStatsByAgentId = `-- name: SentimentStatsByAgentId :one
SELECT
    sum(positive_count) AS positive,
    sum(neutral_count) AS neutral,
    sum(negative_count) AS negative,
    sum(total) AS total
FROM conversation_analytics_hourly
WHERE bucket >= ?
AND agent_id = ?
`

func (q *Queries) ConversationsSummary(ctx context.Context, arg ConversationsSummaryParams) (ConversationSummaryView, error) {
	var row interface {
		Scan(dest ...any) error
	}
	if arg.AgentID != nil {
		row = q.conn.QueryRow(ctx, conversationsSummaryByAgentId, arg.Since, *arg.AgentID)
	} else {
		row = q.conn.QueryRow(ctx, conversationsSummary, arg.Since)
	}

	var (
		total                 uint64
		resolvedCount         uint64
		escalatedCount        uint64
		timedOutCount         uint64
		interceptedCount      uint64
		resolutionMsSum       int64
		uniqUsers             uint64
		positiveCount         uint64
		neutralCount          uint64
		negativeCount         uint64
		otherSentimentCount   uint64
		messageCountSum       int64
		firstResponseMsSum    int64
		firstResponseCount    uint64
		userSatisfactionSum   int64
		userSatisfactionCount uint64
	)
	if err := row.Scan(
		&total,
		&resolvedCount,
		&escalatedCount,
		&timedOutCount,
		&interceptedCount,
		&resolutionMsSum,
		&uniqUsers,
		&positiveCount,
		&neutralCount,
		&negativeCount,
		&otherSentimentCount,
		&messageCountSum,
		&firstResponseMsSum,
		&firstResponseCount,
		&userSatisfactionSum,
		&userSatisfactionCount,
	); err != nil {
		return ConversationSummaryView{}, err
	}

	var resolutionRate, escalationRate, avgResolutionMs, avgMessages float64
	if total > 0 {
		resolutionRate = float64(resolvedCount) / float64(total)
		escalationRate = float64(escalatedCount) / float64(total)
		avgResolutionMs = float64(resolutionMsSum) / float64(total)
		avgMessages = float64(messageCountSum) / float64(total)
	}

	var avgFirstResponseMs float64
	if firstResponseCount > 0 {
		avgFirstResponseMs = float64(firstResponseMsSum) / float64(firstResponseCount)
	}

	var avgUserSatisfaction float64
	if userSatisfactionCount > 0 {
		avgUserSatisfaction = float64(userSatisfactionSum) / float64(userSatisfactionCount)
	}

	return ConversationSummaryView{
		Total:                      int64(total),
		ResolvedCount:              int64(resolvedCount),
		EscalatedCount:             int64(escalatedCount),
		TimedOutCount:              int64(timedOutCount),
		InterceptedCount:           int64(interceptedCount),
		ResolutionRate:             resolutionRate,
		EscalationRate:             escalationRate,
		AvgResolutionMs:            avgResolutionMs,
		AvgFirstResponseMs:         avgFirstResponseMs,
		AvgUserSatisfaction:        avgUserSatisfaction,
		AvgMessagesPerConversation: avgMessages,
		UniqUsers:                  int64(uniqUsers),
		PositiveCount:              int64(positiveCount),
		NeutralCount:               int64(neutralCount),
		NegativeCount:              int64(negativeCount),
		OtherSentimentCount:        int64(otherSentimentCount),
	}, nil
}

func (q *Queries) ConversationsTimeseries(ctx context.Context, arg ConversationsTimeseriesParams) ([]ConversationTimeseriesPoint, error) {
	now := time.Now().UTC()

	var fillFrom, fillTo time.Time
	var query string
	var args []any

	switch arg.Step {
	case "hour":
		fillFrom = arg.Since.Truncate(time.Hour)
		fillTo = now.Truncate(time.Hour).Add(time.Hour)
		if arg.AgentID != nil {
			query, args = conversationsTimeseriesHourlyByAgentId, []any{arg.Since, *arg.AgentID, fillFrom, fillTo}
		} else {
			query, args = conversationsTimeseriesHourly, []any{arg.Since, fillFrom, fillTo}
		}
	case "week":
		fillFrom = truncateToMonday(arg.Since)
		fillTo = truncateToMonday(now).AddDate(0, 0, 7)
		if arg.AgentID != nil {
			query, args = conversationsTimeseriesWeeklyByAgentId, []any{arg.Since, *arg.AgentID, fillFrom, fillTo}
		} else {
			query, args = conversationsTimeseriesWeekly, []any{arg.Since, fillFrom, fillTo}
		}
	default:
		fillFrom = truncateToDay(arg.Since)
		fillTo = truncateToDay(now).AddDate(0, 0, 1)
		if arg.AgentID != nil {
			query, args = conversationsTimeseriesDailyByAgentId, []any{arg.Since, *arg.AgentID, fillFrom, fillTo}
		} else {
			query, args = conversationsTimeseriesDaily, []any{arg.Since, fillFrom, fillTo}
		}
	}

	rows, err := q.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ConversationTimeseriesPoint
	for rows.Next() {
		var (
			bucket         time.Time
			total          uint64
			resolvedCount  uint64
			escalatedCount uint64
			avgResMs       float64
		)
		if err := rows.Scan(&bucket, &total, &resolvedCount, &escalatedCount, &avgResMs); err != nil {
			return nil, err
		}
		var resRate float64
		if total > 0 {
			resRate = float64(resolvedCount) / float64(total)
		}
		items = append(items, ConversationTimeseriesPoint{
			Bucket:          bucket,
			Total:           int64(total),
			ResolvedCount:   int64(resolvedCount),
			EscalatedCount:  int64(escalatedCount),
			ResolutionRate:  resRate,
			AvgResolutionMs: avgResMs,
		})
	}
	return items, rows.Err()
}

func (q *Queries) IntentStats(ctx context.Context, arg IntentStatsParams) ([]IntentStatRow, error) {
	var query string
	var args []any
	if arg.AgentID != nil {
		query, args = intentStatsByAgentId, []any{arg.Since, *arg.AgentID, arg.Limit}
	} else {
		query, args = intentStats, []any{arg.Since, arg.Limit}
	}

	rows, err := q.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []IntentStatRow
	for rows.Next() {
		var (
			intent        string
			total         uint64
			resolvedCount uint64
		)
		if err := rows.Scan(&intent, &total, &resolvedCount); err != nil {
			return nil, err
		}
		var resRate float64
		if total > 0 {
			resRate = float64(resolvedCount) / float64(total)
		}
		items = append(items, IntentStatRow{
			Intent:         intent,
			Total:          int64(total),
			ResolvedCount:  int64(resolvedCount),
			ResolutionRate: resRate,
		})
	}
	return items, rows.Err()
}

func (q *Queries) TopicStats(ctx context.Context, arg TopicStatsParams) ([]TopicStatRow, error) {
	var query string
	var args []any
	if arg.AgentID != nil {
		query, args = topicStatsByAgentId, []any{arg.Since, *arg.AgentID, arg.Limit}
	} else {
		query, args = topicStats, []any{arg.Since, arg.Limit}
	}

	rows, err := q.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TopicStatRow
	for rows.Next() {
		var (
			topic         string
			total         uint64
			resolvedCount uint64
		)
		if err := rows.Scan(&topic, &total, &resolvedCount); err != nil {
			return nil, err
		}
		var resRate float64
		if total > 0 {
			resRate = float64(resolvedCount) / float64(total)
		}
		items = append(items, TopicStatRow{
			Topic:          topic,
			Total:          int64(total),
			ResolvedCount:  int64(resolvedCount),
			ResolutionRate: resRate,
		})
	}
	return items, rows.Err()
}

func (q *Queries) SentimentStats(ctx context.Context, arg SentimentStatsParams) ([]SentimentStatRow, error) {
	var row interface {
		Scan(dest ...any) error
	}
	if arg.AgentID != nil {
		row = q.conn.QueryRow(ctx, sentimentStatsByAgentId, arg.Since, *arg.AgentID)
	} else {
		row = q.conn.QueryRow(ctx, sentimentStats, arg.Since)
	}

	var positive, neutral, negative, total uint64
	if err := row.Scan(&positive, &neutral, &negative, &total); err != nil {
		return nil, err
	}

	pct := func(n uint64) float64 {
		if total == 0 {
			return 0
		}
		return float64(n) / float64(total)
	}

	return []SentimentStatRow{
		{Sentiment: "positive", Total: int64(positive), Pct: pct(positive)},
		{Sentiment: "neutral", Total: int64(neutral), Pct: pct(neutral)},
		{Sentiment: "negative", Total: int64(negative), Pct: pct(negative)},
	}, nil
}

func truncateToMonday(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, time.UTC)
}
