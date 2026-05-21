package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentHandler struct {
	Queries db.Querier
}

func NewAgentHandler(conn *pgxpool.Pool) *AgentHandler {
	return &AgentHandler{Queries: db.New(conn)}
}

type createAgentRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
	WebhookUri  string  `json:"webhook_uri"`
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "webhook_uri are required")
		return
	}

	fmt.Printf("%v", req.Name)
	fmt.Printf("%v", req.IsActive)
	agent, err := h.Queries.InsertAgent(r.Context(), db.InsertAgentParams{
		Name:        req.Name,
		Description: req.Description,
		IsActive:    lib.NullBoolean(req.IsActive),
		WebhookUri:  req.WebhookUri,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

func (h *AgentHandler) Read(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	isActive := lib.ParseParamsBool(params, "is_active")

	agents, err := h.Queries.SelectAgents(r.Context(), db.SelectAgentsParams{
		IsActive: isActive,
		Sort:     pagination.Sort,
		Limit:    pagination.Limit,
		Offset:   pagination.Offset * pagination.Limit,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgents(r.Context())
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	fmt.Printf("%v", len(agents))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

type updateAgentRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	IsActive    bool    `json:"is_active"`
	WebhookUri  string  `json:"webhook_uri"`
}

func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateAgentRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "webhook_uri are required")
		return
	}

	agent, err := h.Queries.UpdateAgent(r.Context(), db.UpdateAgentParams{
		Name:        req.Name,
		Description: req.Description,
		IsActive:    req.IsActive,
		WebhookUri:  req.WebhookUri,
		ID:          id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteAgent(r.Context(), id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
