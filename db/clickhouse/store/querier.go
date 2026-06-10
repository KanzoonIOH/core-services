package store

import "context"

type Querier interface {
	// webhook messages
	CountMessages(ctx context.Context) (int64, error)
	CountMessagesByAgentId(ctx context.Context, agentID string) (int64, error)
	SelectMessages(ctx context.Context, arg SelectMessagesParams) ([]WebhookMessagesView, error)
	SelectMessagesByAgentId(ctx context.Context, arg SelectMessagesByAgentIdParams) ([]WebhookMessagesView, error)
	MessagesSummary(ctx context.Context, arg MessagesSummaryParams) (SummaryView, error)
	MessagesTimeseries(ctx context.Context, arg MessagesTimeseriesParams) ([]TimeseriesPointView, error)
	AgentPerformance(ctx context.Context, arg AgentPerformanceParams) ([]AgentPerformanceRow, error)
	TrafficHeatmap(ctx context.Context, arg TrafficHeatmapParams) ([]TrafficHeatmapRow, error)

	// conversation analytics
	ConversationsSummary(ctx context.Context, arg ConversationsSummaryParams) (ConversationSummaryView, error)
	ConversationsTimeseries(ctx context.Context, arg ConversationsTimeseriesParams) ([]ConversationTimeseriesPoint, error)
	IntentStats(ctx context.Context, arg IntentStatsParams) ([]IntentStatRow, error)
	TopicStats(ctx context.Context, arg TopicStatsParams) ([]TopicStatRow, error)
	SentimentStats(ctx context.Context, arg SentimentStatsParams) ([]SentimentStatRow, error)
}

var _ Querier = (*Queries)(nil)
