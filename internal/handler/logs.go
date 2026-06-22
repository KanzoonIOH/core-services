package handler

import (
	"aic3-service/db/clickhouse/store"
	"aic3-service/internal/lib"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type LogHandler struct {
	Queries store.Querier
}

func NewLogHandler(client *lib.ClickHouseClient) *LogHandler {
	return &LogHandler{Queries: store.New(client.Conn())}
}

func (h *LogHandler) ReadMessages(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	messages, err := h.Queries.SelectMessages(r.Context(), store.SelectMessagesParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}

	totalRow, err := h.Queries.CountMessages(r.Context())
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, messages, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(messages), int(totalRow)))
}

// rangeSince maps a dashboard range param to the inclusive lower bound for
// `bucket >= since`, and reports the timeseries bucket step ("hour" or "day").
// Defaults to "day" (today, hourly step) for unknown/empty values.
func rangeSince(rng string) (since time.Time, step string) {
	now := time.Now().UTC()
	switch rng {
	case "week":
		return now.AddDate(0, 0, -7), "day"
	case "month":
		return now.AddDate(0, -1, 0), "day"
	case "3months":
		return now.AddDate(0, -3, 0), "week"
	default: // "day"
		return now.Truncate(24 * time.Hour), "hour"
	}
}

// parseAgentID reads the optional ?agent_id= query param. Returns (nil, true)
// when absent (global view), (ptr, true) when a valid UUID, and (nil, false)
// when present-but-malformed (a 400 has already been written).
func parseAgentID(w http.ResponseWriter, r *http.Request) (*string, bool) {
	raw := lib.ParseParamsString(r.URL.Query(), "agent_id")
	if raw == nil {
		return nil, true
	}
	if _, err := uuid.Parse(*raw); err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return nil, false
	}
	return raw, true
}

func (h *LogHandler) Summary(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, _ := rangeSince(r.URL.Query().Get("range"))

	summary, err := h.Queries.MessagesSummary(r.Context(), store.MessagesSummaryParams{
		Since:   since,
		AgentID: agentID,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get summary")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, summary, nil)
}

func (h *LogHandler) Timeseries(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, step := rangeSince(r.URL.Query().Get("range"))

	points, err := h.Queries.MessagesTimeseries(r.Context(), store.MessagesTimeseriesParams{
		Since:   since,
		AgentID: agentID,
		Step:    step,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get timeseries")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, points, nil)
}

func (h *LogHandler) AgentPerformance(w http.ResponseWriter, r *http.Request) {
	since, _ := rangeSince(r.URL.Query().Get("range"))

	items, err := h.Queries.AgentPerformance(r.Context(), store.AgentPerformanceParams{
		Since: since,
		Limit: parseTopN(r),
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent performance")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, items, nil)
}

func (h *LogHandler) TrafficHeatmap(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, _ := rangeSince(r.URL.Query().Get("range"))

	items, err := h.Queries.TrafficHeatmap(r.Context(), store.TrafficHeatmapParams{
		Since:   since,
		AgentID: agentID,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get traffic heatmap")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, items, nil)
}

func (h *LogHandler) ReadMessagesByAgentId(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	messages, err := h.Queries.SelectMessagesByAgentId(r.Context(), store.SelectMessagesByAgentIdParams{
		AgentID: id.String(),
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}

	totalRow, err := h.Queries.CountMessagesByAgentId(r.Context(), id.String())
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, messages, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(messages), int(totalRow)))
}

// ---------------------------------------------------------------------------
// Conversation analytics handlers
// ---------------------------------------------------------------------------

// ConversationsSummary handles GET /api/logs/conversations/summary
// Optional: ?agent_id=, ?range=day|week|month
func (h *LogHandler) ConversationsSummary(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, _ := rangeSince(r.URL.Query().Get("range"))

	summary, err := h.Queries.ConversationsSummary(r.Context(), store.ConversationsSummaryParams{
		AgentID: agentID,
		Since:   since,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversations summary")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, summary, nil)
}

// ConversationsTimeseries handles GET /api/logs/conversations/timeseries
// Optional: ?agent_id=, ?range=day|week|month
func (h *LogHandler) ConversationsTimeseries(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, step := rangeSince(r.URL.Query().Get("range"))

	points, err := h.Queries.ConversationsTimeseries(r.Context(), store.ConversationsTimeseriesParams{
		AgentID: agentID,
		Since:   since,
		Step:    step,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversations timeseries")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, points, nil)
}

// parseTopN reads the optional ?limit= query param for top-N breakdowns.
// Defaults to 10, capped at 50.
func parseTopN(r *http.Request) int32 {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10
	}
	if n > 50 {
		return 50
	}
	return int32(n)
}

// IntentStats handles GET /api/logs/conversations/intents
// Optional: ?agent_id=, ?range=, ?limit=N (default 10, max 50)
func (h *LogHandler) IntentStats(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, _ := rangeSince(r.URL.Query().Get("range"))

	stats, err := h.Queries.IntentStats(r.Context(), store.IntentStatsParams{
		AgentID: agentID,
		Since:   since,
		Limit:   parseTopN(r),
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get intent stats")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, stats, nil)
}

// TopicStats handles GET /api/logs/conversations/topics
// Optional: ?agent_id=, ?range=, ?limit=N (default 10, max 50)
func (h *LogHandler) TopicStats(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, _ := rangeSince(r.URL.Query().Get("range"))

	stats, err := h.Queries.TopicStats(r.Context(), store.TopicStatsParams{
		AgentID: agentID,
		Since:   since,
		Limit:   parseTopN(r),
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get topic stats")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, stats, nil)
}

// SentimentStats handles GET /api/logs/conversations/sentiment
// Optional: ?agent_id=, ?range=
func (h *LogHandler) SentimentStats(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	since, _ := rangeSince(r.URL.Query().Get("range"))

	stats, err := h.Queries.SentimentStats(r.Context(), store.SentimentStatsParams{
		AgentID: agentID,
		Since:   since,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get sentiment stats")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, stats, nil)
}
