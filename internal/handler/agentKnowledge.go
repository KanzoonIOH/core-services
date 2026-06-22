package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentKnowledgeHandler struct {
	Queries db.Querier
}

func NewAgentKnowledgeHandler(conn *pgxpool.Pool) *AgentKnowledgeHandler {
	return &AgentKnowledgeHandler{Queries: db.New(conn)}
}

type createAgentKnowledgeRequest struct {
	KnowledgeId uuid.UUID `json:"knowledge_id"`
}

func (h *AgentKnowledgeHandler) CreateByAgentId(w http.ResponseWriter, r *http.Request) {
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req createAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	agent_knowledge, err := h.Queries.InsertAgentKnowledge(r.Context(), db.InsertAgentKnowledgeParams{
		AgentID:     agent_id,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to assign knowledge to agent")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledge, nil)
}

func (h *AgentKnowledgeHandler) ReadByAgentId(w http.ResponseWriter, r *http.Request) {
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	agent_knowledges, err := h.Queries.SelectAgentKnowledgesByAgentId(r.Context(), db.SelectAgentKnowledgesByAgentIdParams{
		AgentID: agent_id,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent knowledges")
		return
	}

	totalRow, err := h.Queries.CountAgentKnowledgesByAgentId(r.Context(), agent_id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent knowledges")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agent_knowledges), int(totalRow)))
}

type updateAgentKnowledgeRequest struct {
	IsActiveProd bool `json:"is_active_prod"`
	IsActiveDev  bool `json:"is_active_dev"`
}

func (h *AgentKnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateAgentKnowledgeRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	agent_knowledge, err := h.Queries.UpdateAgentKnowledge(r.Context(), db.UpdateAgentKnowledgeParams{
		IsActiveProd: req.IsActiveProd,
		IsActiveDev:  req.IsActiveDev,
		ID:           id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update agent knowledge")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledge, nil)
}

func (h *AgentKnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteAgentKnowledge(r.Context(), id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete agent knowledge")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
