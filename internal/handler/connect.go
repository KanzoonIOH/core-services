package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConnectHandler struct {
	Queries db.Querier
}

func NewConnectHandler(conn *pgxpool.Pool) *ConnectHandler {
	return &ConnectHandler{Queries: db.New(conn)}
}

type connectAgentMcpRequest struct {
	AgentId uuid.UUID `json:"agent_id"`
	McpId   uuid.UUID `json:"mcp_id"`
}

func (h *ConnectHandler) ConnectAgentMcp(w http.ResponseWriter, r *http.Request) {
	var req connectAgentMcpRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	agent_mcp, err := h.Queries.InsertAgentMcp(r.Context(), db.InsertAgentMcpParams{
		AgentID: req.AgentId,
		McpID:   req.McpId,
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect mcp to agent")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_mcp, nil)
}

func (h *ConnectHandler) DisconnectAgentMcp(w http.ResponseWriter, r *http.Request) {
	var req connectAgentMcpRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteAgentMcp(r.Context(), db.SoftDeleteAgentMcpParams{
		AgentID: req.AgentId,
		McpID:   req.McpId,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to disconnect mcp from agent")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "agent mcp not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

type connectAgentKnowledgeRequest struct {
	AgentId     uuid.UUID `json:"agent_id"`
	KnowledgeId uuid.UUID `json:"knowledge_id"`
}

func (h *ConnectHandler) ConnectAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	agent_knowledge, err := h.Queries.InsertAgentKnowledge(r.Context(), db.InsertAgentKnowledgeParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect knowledge to agent")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledge, nil)
}

func (h *ConnectHandler) DisconnectAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteAgentKnowledgeByPair(r.Context(), db.SoftDeleteAgentKnowledgeByPairParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to disconnect knowledge from agent")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
