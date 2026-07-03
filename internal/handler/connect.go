package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Default knowledge (Milvus/n8n) API base URL, overridable via KNOWLEDGE_API_BASE_URL.
const defaultKnowledgeAPIBaseURL = "https://103.67.43.198:8443/milvus"

type ConnectHandler struct {
	Queries    db.Querier
	HTTPClient *http.Client
	// knowledgeAddURL is POSTed to on connect; knowledgeDeleteURL is a
	// fmt template ("...%s") DELETEd on disconnect.
	knowledgeAddURL    string
	knowledgeDeleteURL string
}

func NewConnectHandler(conn *pgxpool.Pool) *ConnectHandler {
	base := strings.TrimRight(os.Getenv("KNOWLEDGE_API_BASE_URL"), "/")
	if base == "" {
		base = defaultKnowledgeAPIBaseURL
	}

	return &ConnectHandler{
		Queries:            db.New(conn),
		knowledgeAddURL:    base + "/knowledge/add",
		knowledgeDeleteURL: base + "/knowledge/%s", // DELETE /knowledge/{knowledge_id}
		HTTPClient: &http.Client{
			Timeout: 10 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
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
	log.Printf("[core-service][connect-agent-mcp] api hit method=%s path=%s", r.Method, r.URL.Path)
	var req connectAgentMcpRequest

	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][connect-agent-mcp] api error invalid JSON body")
		return
	}
	log.Printf("[core-service][connect-agent-mcp] request params=%s", jsonForLog(req))

	agent_mcp, err := h.Queries.InsertAgentMcp(r.Context(), db.InsertAgentMcpParams{
		AgentID: req.AgentId,
		McpID:   req.McpId,
	})
	if err != nil {
		log.Printf("[core-service][connect-agent-mcp] db insert error agent_id=%s mcp_id=%s error=%v", req.AgentId, req.McpId, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect mcp to agent")
		return
	}

	log.Printf("[core-service][connect-agent-mcp] api success status=%d response=%s", http.StatusOK, jsonForLog(agent_mcp))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_mcp, nil)
}

func (h *ConnectHandler) DisconnectAgentMcp(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][disconnect-agent-mcp] api hit method=%s path=%s", r.Method, r.URL.Path)
	var req connectAgentMcpRequest

	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][disconnect-agent-mcp] api error invalid JSON body")
		return
	}
	log.Printf("[core-service][disconnect-agent-mcp] request params=%s", jsonForLog(req))

	rowsAffected, err := h.Queries.SoftDeleteAgentMcp(r.Context(), db.SoftDeleteAgentMcpParams{
		AgentID: req.AgentId,
		McpID:   req.McpId,
	})
	if err != nil {
		log.Printf("[core-service][disconnect-agent-mcp] db soft-delete error agent_id=%s mcp_id=%s error=%v", req.AgentId, req.McpId, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to disconnect mcp from agent")
		return
	}

	if rowsAffected == 0 {
		log.Printf("[core-service][disconnect-agent-mcp] api error status=%d reason=agent mcp not found params=%s", http.StatusNotFound, jsonForLog(req))
		lib.ResponseJSONError(w, http.StatusNotFound, "agent mcp not found")
		return
	}

	log.Printf("[core-service][disconnect-agent-mcp] api success status=%d rows_affected=%d", http.StatusNoContent, rowsAffected)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

type connectAgentKnowledgeRequest struct {
	AgentId     uuid.UUID `json:"agent_id"`
	KnowledgeId uuid.UUID `json:"knowledge_id"`
}

func (h *ConnectHandler) ConnectAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][connect-agent-knowledge] api hit method=%s path=%s", r.Method, r.URL.Path)
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][connect-agent-knowledge] api error invalid JSON body")
		return
	}
	log.Printf("[core-service][connect-agent-knowledge] request params=%s", jsonForLog(req))

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), req.KnowledgeId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[core-service][connect-agent-knowledge] api error status=%d reason=knowledge not found knowledge_id=%s", http.StatusNotFound, req.KnowledgeId)
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}
		log.Printf("[core-service][connect-agent-knowledge] db select knowledge error knowledge_id=%s error=%v", req.KnowledgeId, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect knowledge to agent")
		return
	}
	log.Printf("[core-service][connect-agent-knowledge] selected knowledge id=%s source_uri=%v", req.KnowledgeId, knowledge.SourceUri)

	agent_knowledge, err := h.Queries.InsertAgentKnowledge(r.Context(), db.InsertAgentKnowledgeParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		log.Printf("[core-service][connect-agent-knowledge] db insert error agent_id=%s knowledge_id=%s error=%v", req.AgentId, req.KnowledgeId, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect knowledge to agent")
		return
	}
	log.Printf("[core-service][connect-agent-knowledge] db insert success agent_knowledge=%s", jsonForLog(agent_knowledge))

	// The RAG collection to ingest into is the agent's own collection. Best
	// effort: if the agent lookup fails, the rag-service falls back to its
	// default collection when collection_name is empty.
	var milvusCollection string
	if agent, err := h.Queries.SelectAgentById(r.Context(), req.AgentId); err != nil {
		log.Printf("[core-service][connect-agent-knowledge] agent lookup for milvus collection failed agent_id=%s error=%v", req.AgentId, err)
	} else {
		milvusCollection = agent.MilvusCollection
	}

	go h.triggerKnowledgeConversion(agent_knowledge.ID, req.AgentId, req.KnowledgeId, knowledge.SourceUri, milvusCollection)
	log.Printf("[core-service][connect-agent-knowledge] rag conversion trigger queued agent_knowledge_id=%s agent_id=%s knowledge_id=%s collection=%s", agent_knowledge.ID, req.AgentId, req.KnowledgeId, milvusCollection)

	log.Printf("[core-service][connect-agent-knowledge] api success status=%d response=%s", http.StatusOK, jsonForLog(agent_knowledge))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledge, nil)
}

// triggerKnowledgeConversion fires the RAG ingestion webhook in the background.
// Ingestion is legitimately long-running, so this is fire-and-forget: it does
// NOT mark the row 'failed' on a slow or dropped connection. The rag-service
// owns the final 'completed'/'failed' status and reports it via the
// agent-knowledge-status callback endpoint. Only a payload we cannot even build
// (which means the row can never complete) is marked failed here.
func (h *ConnectHandler) triggerKnowledgeConversion(agentKnowledgeID, agentID, knowledgeID uuid.UUID, sourceURI *string, milvusCollection string) {
	// Generous ceiling so we don't cancel a long-but-progressing ingestion.
	// The client also has its own 10-minute Timeout as a hard cap.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	payload, err := json.Marshal(map[string]any{
		"agent_knowledge_id": agentKnowledgeID,
		"agent_id":           agentID,
		"document_id":        knowledgeID,
		"document_link":      sourceURI,
		"collection_name":    milvusCollection,
	})
	if err != nil {
		log.Printf("[core-service][rag-knowledge-add] payload marshal error agent_knowledge_id=%s agent_id=%s knowledge_id=%s source_uri=%v collection=%s error=%v", agentKnowledgeID, agentID, knowledgeID, sourceURI, milvusCollection, err)
		h.markKnowledgeConversionFailed(agentKnowledgeID, err)
		return
	}

	log.Printf("[core-service][rag-knowledge-add] fetch start method=%s url=%s params={agent_knowledge_id:%s agent_id:%s knowledge_id:%s source_uri:%v collection:%s} payload=%s", http.MethodPost, h.knowledgeAddURL, agentKnowledgeID, agentID, knowledgeID, sourceURI, milvusCollection, string(payload))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.knowledgeAddURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[core-service][rag-knowledge-add] request build error url=%s payload=%s error=%v", h.knowledgeAddURL, string(payload), err)
		h.markKnowledgeConversionFailed(agentKnowledgeID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := h.HTTPClient.Do(req)
	duration := time.Since(started)
	if err != nil {
		// Fire-and-forget: a slow/dropped hit does NOT fail the row. The
		// rag-service reports the real outcome via its status callback.
		log.Printf("[core-service][rag-knowledge-add] fetch error (ignored, status owned by rag-service) method=%s url=%s duration=%s payload=%s error=%v", http.MethodPost, h.knowledgeAddURL, duration, string(payload), err)
		return
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	bodyText := truncateForLog(string(body), 4096)
	if readErr != nil {
		log.Printf("[core-service][rag-knowledge-add] response body read error status=%d duration=%s error=%v", resp.StatusCode, duration, readErr)
	}

	log.Printf("[core-service][rag-knowledge-add] fetch returned method=%s url=%s status=%d duration=%s response_body=%q", http.MethodPost, h.knowledgeAddURL, resp.StatusCode, duration, bodyText)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx is logged but not marked failed here either — the
		// rag-service already reports 'failed' via callback on its errors.
		log.Printf("[core-service][rag-knowledge-add] fetch non-2xx (ignored, status owned by rag-service) agent_knowledge_id=%s status=%d body=%q", agentKnowledgeID, resp.StatusCode, bodyText)
		return
	}

	log.Printf("[core-service][rag-knowledge-add] fetch success agent_knowledge_id=%s status=%d body=%q", agentKnowledgeID, resp.StatusCode, bodyText)
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
	log.Printf("[core-service][disconnect-agent-knowledge] api hit method=%s path=%s", r.Method, r.URL.Path)
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][disconnect-agent-knowledge] api error invalid JSON body")
		return
	}
	log.Printf("[core-service][disconnect-agent-knowledge] request params=%s", jsonForLog(req))

	agentKnowledgeID, err := h.Queries.SoftDeleteAgentKnowledgeByPair(r.Context(), db.SoftDeleteAgentKnowledgeByPairParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[core-service][disconnect-agent-knowledge] api error status=%d reason=agent knowledge not found params=%s", http.StatusNotFound, jsonForLog(req))
			lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
			return
		}
		log.Printf("[core-service][disconnect-agent-knowledge] db soft-delete error agent_id=%s knowledge_id=%s error=%v", req.AgentId, req.KnowledgeId, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to disconnect knowledge from agent")
		return
	}

	// Look up source_uri (document_link) so the delete body matches connect's
	// exactly. Best effort: empty on lookup failure.
	var sourceURI *string
	if knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), req.KnowledgeId); err != nil {
		log.Printf("[core-service][disconnect-agent-knowledge] knowledge lookup for source_uri failed knowledge_id=%s error=%v", req.KnowledgeId, err)
	} else {
		sourceURI = knowledge.SourceUri
	}

	// Same collection resolution as connect: delete from the agent's own
	// collection. Empty = rag-service falls back to its default collection.
	var milvusCollection string
	if agent, err := h.Queries.SelectAgentById(r.Context(), req.AgentId); err != nil {
		log.Printf("[core-service][disconnect-agent-knowledge] agent lookup for milvus collection failed agent_id=%s error=%v", req.AgentId, err)
	} else {
		milvusCollection = agent.MilvusCollection
	}

	go h.triggerKnowledgeDeletion(agentKnowledgeID, req.AgentId, req.KnowledgeId, sourceURI, milvusCollection)
	log.Printf("[core-service][disconnect-agent-knowledge] rag deletion trigger queued agent_knowledge_id=%s agent_id=%s knowledge_id=%s collection=%s", agentKnowledgeID, req.AgentId, req.KnowledgeId, milvusCollection)

	log.Printf("[core-service][disconnect-agent-knowledge] api success status=%d", http.StatusNoContent)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

// triggerKnowledgeDeletion fires the knowledge REST API delete endpoint in the
// background. Failures are logged only; the local disconnect has already
// succeeded by the time this runs.
func (h *ConnectHandler) triggerKnowledgeDeletion(agentKnowledgeID, agentID, knowledgeID uuid.UUID, sourceURI *string, milvusCollection string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf(h.knowledgeDeleteURL, knowledgeID)
	// Identical body shape to connect (triggerKnowledgeConversion).
	payload, err := json.Marshal(map[string]any{
		"agent_knowledge_id": agentKnowledgeID,
		"agent_id":           agentID,
		"document_id":        knowledgeID,
		"document_link":      sourceURI,
		"collection_name":    milvusCollection,
	})
	if err != nil {
		log.Printf("[core-service][rag-knowledge-delete] payload marshal error knowledge_id=%s collection=%s error=%v", knowledgeID, milvusCollection, err)
		return
	}
	log.Printf("[core-service][rag-knowledge-delete] fetch start method=%s url=%s params={knowledge_id:%s collection:%s} payload=%s", http.MethodDelete, url, knowledgeID, milvusCollection, string(payload))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[core-service][rag-knowledge-delete] request build error knowledge_id=%s url=%s error=%v", knowledgeID, url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := h.HTTPClient.Do(req)
	duration := time.Since(started)
	if err != nil {
		log.Printf("[core-service][rag-knowledge-delete] fetch error method=%s url=%s duration=%s knowledge_id=%s error=%v", http.MethodDelete, url, duration, knowledgeID, err)
		return
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	bodyText := truncateForLog(string(body), 4096)
	if readErr != nil {
		log.Printf("[core-service][rag-knowledge-delete] response body read error status=%d duration=%s error=%v", resp.StatusCode, duration, readErr)
	}

	log.Printf("[core-service][rag-knowledge-delete] fetch returned method=%s url=%s status=%d duration=%s response_body=%q", http.MethodDelete, url, resp.StatusCode, duration, bodyText)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[core-service][rag-knowledge-delete] fetch failed knowledge_id=%s status=%d reason=API returned non-2xx body=%q", knowledgeID, resp.StatusCode, bodyText)
		return
	}

	log.Printf("[core-service][rag-knowledge-delete] fetch success knowledge_id=%s status=%d body=%q", knowledgeID, resp.StatusCode, bodyText)
}
