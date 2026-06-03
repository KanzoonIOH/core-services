package handler

import (
	"aiac-service/db/clickhouse/store"
	"aiac-service/internal/lib"
	"net/http"
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
