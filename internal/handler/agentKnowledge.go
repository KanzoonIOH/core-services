package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
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
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to assign knowledge to agent")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agent_knowledge)
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
		Limit:   pagination.Limit,
		Offset:  pagination.Offset,
	})
	if err != nil {
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get agent knowledges")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agent_knowledges)

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
			lib.ResponseJSON(w, http.StatusNotFound, "agent knowledge not found")
			return
		}

		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to update agent knowledge")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, agent_knowledge)
}

func (h *AgentKnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.DeleteAgentKnowledge(r.Context(), id)
	if err != nil {
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to delete agent knowledge")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSON(w, http.StatusNotFound, "agent knowledge not found")
		return
	}

	lib.ResponseJSON(w, http.StatusNoContent, nil)
}
