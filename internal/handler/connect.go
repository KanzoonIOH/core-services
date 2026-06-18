package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TODO: move to env config once the n8n workflow is finalized.
const (
	knowledgeAPIBaseURL = "https://103.67.43.198:8443/milvus"
	knowledgeAddURL     = knowledgeAPIBaseURL + "/knowledge/add"
	knowledgeDeleteURL  = knowledgeAPIBaseURL + "/knowledge/%s" // DELETE /knowledge/{knowledge_id}
)

type ConnectHandler struct {
	Queries    db.Querier
	HTTPClient *http.Client
}

func NewConnectHandler(conn *pgxpool.Pool) *ConnectHandler {
	return &ConnectHandler{
		Queries: db.New(conn),
		HTTPClient: &http.Client{
			Timeout: 10 * time.Minute,
			Transport: &hostTLSBypassTransport{
				secure: http.DefaultTransport,
				insecure: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
		},
	}
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

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), req.KnowledgeId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect knowledge to agent")
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

	go h.triggerKnowledgeConversion(agent_knowledge.ID, req.AgentId, req.KnowledgeId, knowledge.SourceUri)

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledge, nil)
}

// triggerKnowledgeConversion fires the n8n conversion workflow webhook in the
// background. It only reports trigger delivery: if the webhook cannot be
// reached or rejects the request, the row is marked 'failed'. The final
// 'completed'/'failed' status is reported by the workflow itself via the
// agent-knowledge-status callback endpoint.
func (h *ConnectHandler) triggerKnowledgeConversion(agentKnowledgeID, agentID, knowledgeID uuid.UUID, sourceURI *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	payload, err := json.Marshal(map[string]any{
		// "agent_knowledge_id": agentKnowledgeID,
		// "agent_id":           agentID,
		"document_id":   knowledgeID,
		"document_link": sourceURI,
	})
	if err != nil {
		h.markKnowledgeConversionFailed(agentKnowledgeID, err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, knowledgeAddURL, bytes.NewReader(payload))
	if err != nil {
		h.markKnowledgeConversionFailed(agentKnowledgeID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		h.markKnowledgeConversionFailed(agentKnowledgeID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.markKnowledgeConversionFailed(agentKnowledgeID, fmt.Errorf("webhook returned status %d", resp.StatusCode))
	}
}

func (h *ConnectHandler) markKnowledgeConversionFailed(agentKnowledgeID uuid.UUID, cause error) {
	log.Printf("knowledge conversion trigger failed for agent_knowledge %s: %v", agentKnowledgeID, cause)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := h.Queries.UpdateAgentKnowledgeStatus(ctx, db.UpdateAgentKnowledgeStatusParams{
		Status: "failed",
		ID:     agentKnowledgeID,
	}); err != nil {
		log.Printf("failed to mark agent_knowledge %s as failed: %v", agentKnowledgeID, err)
	}
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

	go h.triggerKnowledgeDeletion(req.KnowledgeId)

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

// triggerKnowledgeDeletion fires the knowledge REST API delete endpoint in the
// background. Failures are logged only; the local disconnect has already
// succeeded by the time this runs.
func (h *ConnectHandler) triggerKnowledgeDeletion(knowledgeID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf(knowledgeDeleteURL, knowledgeID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		log.Printf("knowledge deletion trigger failed for knowledge %s: %v", knowledgeID, err)
		return
	}

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		log.Printf("knowledge deletion trigger failed for knowledge %s: %v", knowledgeID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("knowledge deletion trigger failed for knowledge %s: API returned status %d", knowledgeID, resp.StatusCode)
	}
}
