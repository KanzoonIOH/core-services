package store

import "context"

type Querier interface {
	CountMessages(ctx context.Context) (int64, error)
	CountMessagesByAgentId(ctx context.Context, agentID string) (int64, error)
	SelectMessages(ctx context.Context, arg SelectMessagesParams) ([]WebhookMessagesView, error)
	SelectMessagesByAgentId(ctx context.Context, arg SelectMessagesByAgentIdParams) ([]WebhookMessagesView, error)
	MessagesSummary(ctx context.Context, arg MessagesSummaryParams) (SummaryView, error)
	MessagesTimeseries(ctx context.Context, arg MessagesTimeseriesParams) ([]TimeseriesPointView, error)
}

var _ Querier = (*Queries)(nil)
