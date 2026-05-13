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

type createRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
	WebhookUri  string  `json:"webhook_uri"`
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "webhook_uri are required")
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
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agent)
}

func (h *AgentHandler) Read(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	isActive := lib.ParseQueryBool(params, "is_active")

	agents, err := h.Queries.SelectAgents(r.Context(), db.SelectAgentsParams{
		IsActive: isActive,
		Sort:     pagination.Sort,
		Limit:    pagination.Limit,
		Offset:   pagination.Offset,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agents)
}

func (h *AgentHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSON(w, http.StatusNotFound, "agent not found")
			return
		}

		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agent)
}

type updateRequest struct {
	createRequest
	IsActive bool `json:"is_active"`
}

func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "webhook_uri are required")
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
			lib.ResponseJSON(w, http.StatusNotFound, "agent not found")
			return
		}

		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agent)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.DeleteAgent(r.Context(), id)
	if err != nil {
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSON(w, http.StatusNotFound, "agent not found")
		return
	}

	lib.ResponseJSON(w, http.StatusNoContent, nil)
}

// func (h *AgentHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
// 	id, ok := lib.ParseID(w, r, "id")
// 	if !ok {
// 		return
// 	}

// 	agent, err := h.Queries.DuplicateAgent(r.Context(), id)
// 	if err != nil {
// 		if errors.Is(err, pgx.ErrNoRows) {
// 			lib.ResponseError(w, http.StatusNotFound, "agent not found")
// 			return
// 		}

// 		lib.ResponseError(w, http.StatusInternalServerError, "failed to duplicate agent")
// 		return
// 	}

// 	lib.ResponseJSON(w, http.StatusOK, agent)
// }

// func (h *AgentHandler) ReadIdKnowledges(w http.ResponseWriter, r *http.Request) {
// 	id, ok := lib.ParseID(w, r, "id")
// 	if !ok {
// 		return
// 	}

// 	knowledges, err := h.Queries.GetAgentKnowledges(r.Context(), id)
// 	if err != nil {
// 		lib.ResponseError(w, http.StatusInternalServerError, "failed to get agent knowledges")
// 		return
// 	}

// 	lib.ResponseJSON(w, http.StatusOK, knowledges)
// }

// func (h *AgentHandler) ReadIdMcps(w http.ResponseWriter, r *http.Request) {
// 	id, ok := lib.ParseID(w, r, "id")
// 	if !ok {
// 		return
// 	}

// 	mcps, err := h.Queries.GetAgentMcps(r.Context(), id)
// 	if err != nil {
// 		lib.ResponseError(w, http.StatusInternalServerError, "failed to get agent mcps")
// 		return
// 	}

// 	lib.ResponseJSON(w, http.StatusOK, mcps)
// }
