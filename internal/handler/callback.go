package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CallbackHandler struct {
	Queries db.Querier
}

func NewCallbackHandler(conn *pgxpool.Pool) *CallbackHandler {
	return &CallbackHandler{Queries: db.New(conn)}
}

type agentKnowledgeStatusRequest struct {
	Id     uuid.UUID `json:"agent_knowledge_id"`
	Status string    `json:"status"`
}

func (h *CallbackHandler) UpdateAgentKnowledgeStatus(w http.ResponseWriter, r *http.Request) {
	var req agentKnowledgeStatusRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	if req.Status != "completed" && req.Status != "failed" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "status must be 'completed' or 'failed'")
		return
	}

	rowsAffected, err := h.Queries.UpdateAgentKnowledgeStatus(r.Context(), db.UpdateAgentKnowledgeStatusParams{
		Status: req.Status,
		ID:     req.Id,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update agent knowledge status")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, req, nil)
}
