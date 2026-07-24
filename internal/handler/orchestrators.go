package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Default orchestrator API base URL, overridable via ORCHESTRATOR_API_BASE_URL.
const defaultOrchestratorAPIBaseURL = "https://103.67.43.198:8443"

type OrchestratorHandler struct {
	Queries    db.Querier
	HTTPClient *http.Client
	createURL  string
}

func NewOrchestratorHandler(conn *pgxpool.Pool) *OrchestratorHandler {
	base := strings.TrimRight(os.Getenv("ORCHESTRATOR_API_BASE_URL"), "/")
	if base == "" {
		base = defaultOrchestratorAPIBaseURL
	}
	return &OrchestratorHandler{
		Queries:   db.New(conn),
		createURL: base + "/agents/createOrchestrate",
		HTTPClient: &http.Client{
			Timeout: 2 * time.Minute,
			Transport: &hostTLSBypassTransport{
				secure: http.DefaultTransport,
				insecure: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
		},
	}
}

// ---------- Create ----------

type createOrchestratorRequest struct {
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	IsActive    *bool       `json:"is_active"`
	Tags        []string    `json:"tags"` // accepted for parity with agents; not stored yet
	AgentIDs    []uuid.UUID `json:"agent_ids"`
}

// upstream request/response for POST /agents/orchestrator/create.
type orchestratorCreateUpstreamAgent struct {
	AgentID  string `json:"agent_id"`
	Endpoint string `json:"endpoint"`
}

type orchestratorCreateUpstreamReq struct {
	Name   string                            `json:"name"`
	Agents []orchestratorCreateUpstreamAgent `json:"agents"`
}

type orchestratorCreateUpstreamRespAgent struct {
	AgentID     string `json:"agent_id"`
	Endpoint    string `json:"endpoint"`
	ToolName    string `json:"tool_name"`
	Description string `json:"description"`
	CanAct      bool   `json:"can_act"`
}

type orchestratorCreateUpstreamResp struct {
	OrchestratorID      string                                `json:"orchestrator_id"`
	OrchestratorAgentID string                                `json:"orchestrator_agent_id"`
	Name                string                                `json:"name"`
	RoutingGuide        string                                `json:"routing_guide"`
	Persona             string                                `json:"persona"`
	Guardrail           string                                `json:"guardrail"`
	Agents              []orchestratorCreateUpstreamRespAgent `json:"agents"`
}

func (h *OrchestratorHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "orchestrators: create api hit", "method", r.Method, "path", r.URL.Path)
	var req createOrchestratorRequest
	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "orchestrators: create invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "orchestrators: create request", "params", jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if len(req.AgentIDs) == 0 {
		lib.ResponseJSONError(w, http.StatusBadRequest, "at least one agent is required")
		return
	}

	// Resolve each selected agent to its endpoint (== template_id). This is the
	// only place endpoint comes from, so a blank template_id is rejected. Also
	// dedupes agent ids by endpoint uniqueness at the DB layer later.
	upstreamAgents := make([]orchestratorCreateUpstreamAgent, 0, len(req.AgentIDs))
	// endpoint -> our agent uuid, to match the upstream response back to us.
	agentByEndpoint := map[string]uuid.UUID{}
	// agent_id -> endpoint, so the mapping loop can iterate the user's selection
	// directly instead of relying on the upstream response (which may drop one).
	resolvedAgents := map[uuid.UUID]string{}
	seen := map[uuid.UUID]bool{}
	for _, id := range req.AgentIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		agent, err := h.Queries.SelectAgentById(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				lib.ResponseJSONError(w, http.StatusBadRequest, "agent not found: "+id.String())
				return
			}
			slog.ErrorContext(r.Context(), "orchestrators: create agent lookup failed", "agent_id", id, "error", err)
			lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to resolve agents")
			return
		}
		endpoint := strings.TrimSpace(agent.TemplateID)
		if endpoint == "" {
			lib.ResponseJSONError(w, http.StatusBadRequest, "agent "+agent.Name+" has no template/endpoint")
			return
		}
		upstreamAgents = append(upstreamAgents, orchestratorCreateUpstreamAgent{
			AgentID:  id.String(),
			Endpoint: endpoint,
		})
		agentByEndpoint[endpoint] = id
		resolvedAgents[id] = endpoint
	}

	// Call the external orchestrator/create service.
	upstreamResp, err := h.callUpstreamCreate(r.Context(), orchestratorCreateUpstreamReq{
		Name:   req.Name,
		Agents: upstreamAgents,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: create upstream failed", "url", h.createURL, "error", err)
		lib.ResponseJSONError(w, http.StatusBadGateway, "failed to create orchestrator upstream")
		return
	}

	// Persist orchestrator-level data: only routing_guide, persona, guardrail and
	// the orchestrator_agent_id used to call it later.
	orch, err := h.Queries.InsertOrchestrator(r.Context(), db.InsertOrchestratorParams{
		Name:                req.Name,
		Description:         req.Description,
		IsActive:            lib.NullBoolean(req.IsActive),
		OrchestratorAgentID: upstreamResp.OrchestratorAgentID,
		RoutingGuide:        upstreamResp.RoutingGuide,
		Persona:             upstreamResp.Persona,
		Guardrail:           upstreamResp.Guardrail,
		WebhookUri:          "https://103.67.43.198:8443/agents/orchestrator/ask",
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: create db insert failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create orchestrator")
		return
	}

	// Persist per-agent mapping. Iterate the user's selected agent_ids directly
	// (not the upstream response, which may return fewer agents than sent).
	// tool_name/description come from the upstream response, matched by endpoint;
	// if the upstream didn't return an entry for an agent, fall back to empty.
	upstreamByEndpoint := map[string]orchestratorCreateUpstreamRespAgent{}
	for _, ua := range upstreamResp.Agents {
		upstreamByEndpoint[ua.Endpoint] = ua
	}
	for _, id := range req.AgentIDs {
		endpoint, ok := resolvedAgents[id]
		if !ok {
			continue
		}
		ua := upstreamByEndpoint[endpoint]
		if _, err := h.Queries.InsertOrchestratorAgent(r.Context(), db.InsertOrchestratorAgentParams{
			OrchestratorID: orch.ID,
			AgentID:        id,
			ToolName:       ua.ToolName,
			Description:    ua.Description,
		}); err != nil {
			slog.ErrorContext(r.Context(), "orchestrators: create db insert agent mapping failed", "orchestrator_id", orch.ID, "agent_id", id, "error", err)
			lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to link orchestrator agents")
			return
		}
	}

	full, err := h.buildDetail(r.Context(), orch.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: create reload failed", "orchestrator_id", orch.ID, "error", err)
		lib.ResponseJSONTemplate(w, http.StatusOK, nil, orch, nil)
		return
	}

	slog.InfoContext(r.Context(), "orchestrators: create success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

func (h *OrchestratorHandler) callUpstreamCreate(ctx context.Context, body orchestratorCreateUpstreamReq) (*orchestratorCreateUpstreamResp, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "orchestrators: create upstream fetch start", "url", h.createURL, "payload", string(payload))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.createURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	slog.InfoContext(ctx, "orchestrators: create upstream returned", "status", resp.StatusCode, "body", truncateForLog(string(respBytes), 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("upstream non-2xx")
	}

	var out orchestratorCreateUpstreamResp
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------- Read ----------

// orchestratorAgentDetail is the per-agent mapping enriched with the fields we
// re-derive from the agent row instead of storing (endpoint, can_act).
type orchestratorAgentDetail struct {
	AgentID     uuid.UUID `json:"agent_id"`
	AgentName   string    `json:"agent_name"`
	AgentImage  *string   `json:"agent_image"`
	Endpoint    string    `json:"endpoint"`
	ToolName    string    `json:"tool_name"`
	Description string    `json:"description"`
	CanAct      bool      `json:"can_act"`
}

type orchestratorDetail struct {
	db.OrchestratorsView
	Agents []orchestratorAgentDetail `json:"agents"`
}

func (h *OrchestratorHandler) buildDetail(ctx context.Context, id uuid.UUID) (*orchestratorDetail, error) {
	orch, err := h.Queries.SelectOrchestratorById(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := h.Queries.SelectOrchestratorAgents(ctx, id)
	if err != nil {
		return nil, err
	}
	agents := make([]orchestratorAgentDetail, 0, len(rows))
	for _, row := range rows {
		agents = append(agents, orchestratorAgentDetail{
			AgentID:     row.AgentID,
			AgentName:   row.AgentName,
			AgentImage:  row.AgentImage,
			Endpoint:    row.Endpoint,
			ToolName:    row.ToolName,
			Description: row.Description,
			CanAct:      row.CanAct,
		})
	}
	return &orchestratorDetail{OrchestratorsView: orch, Agents: agents}, nil
}

func (h *OrchestratorHandler) Read(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "orchestrators: read api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	isActive := lib.ParseParamsBool(params, "is_active")
	search := lib.ParseParamsString(params, "search")

	orchestrators, err := h.Queries.SelectOrchestrators(r.Context(), db.SelectOrchestratorsParams{
		Search:   search,
		IsActive: isActive,
		Sort:     pagination.Sort,
		Limit:    pagination.Limit,
		Offset:   pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: read db select failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get orchestrators")
		return
	}

	totalRow, err := h.Queries.CountOrchestrators(r.Context(), db.CountOrchestratorsParams{
		Search:   search,
		IsActive: isActive,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: read db count failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get orchestrators")
		return
	}

	slog.InfoContext(r.Context(), "orchestrators: read success", "status", http.StatusOK, "count", len(orchestrators), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, orchestrators, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(orchestrators), int(totalRow)))
}

func (h *OrchestratorHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "orchestrators: read by id api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	full, err := h.buildDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "orchestrator not found")
			return
		}
		slog.ErrorContext(r.Context(), "orchestrators: read by id db failed", "orchestrator_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get orchestrator")
		return
	}

	slog.InfoContext(r.Context(), "orchestrators: read by id success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

// ---------- Update ----------

// Editable orchestrator fields. Sub-agent mapping is set at create time and not
// edited here. persona is a free-text/JSON blob (the frontend serializes its
// structured persona into it), guardrail + routing_guide are free text.
type updateOrchestratorRequest struct {
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	IsActive     *bool   `json:"is_active"`
	RoutingGuide string  `json:"routing_guide"`
	Persona      string  `json:"persona"`
	Guardrail    string  `json:"guardrail"`
	Image        *string `json:"image"`
	WebhookUri   string  `json:"webhook_uri"`
}

func (h *OrchestratorHandler) Update(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "orchestrators: update api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateOrchestratorRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}
	slog.InfoContext(r.Context(), "orchestrators: update request", "orchestrator_id", id, "body", jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}

	if _, err := h.Queries.UpdateOrchestrator(r.Context(), db.UpdateOrchestratorParams{
		Name:         req.Name,
		Description:  req.Description,
		IsActive:     lib.NullBoolean(req.IsActive),
		RoutingGuide: strings.TrimSpace(req.RoutingGuide),
		Persona:      req.Persona,
		Guardrail:    strings.TrimSpace(req.Guardrail),
		Image:        req.Image,
		WebhookUri:   strings.TrimSpace(req.WebhookUri),
		ID:           id,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "orchestrator not found")
			return
		}
		slog.ErrorContext(r.Context(), "orchestrators: update db update failed", "orchestrator_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update orchestrator")
		return
	}

	full, err := h.buildDetail(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: update reload failed", "orchestrator_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update orchestrator")
		return
	}

	slog.InfoContext(r.Context(), "orchestrators: update success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

func (h *OrchestratorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "orchestrators: delete api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteOrchestrator(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "orchestrators: delete db soft-delete failed", "orchestrator_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete orchestrator")
		return
	}
	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "orchestrator not found")
		return
	}

	slog.InfoContext(r.Context(), "orchestrators: delete success", "status", http.StatusNoContent, "rows_affected", rowsAffected)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
