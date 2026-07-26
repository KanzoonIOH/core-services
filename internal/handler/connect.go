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
	neturl "net/url"
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
	// knowledgeAddURL and knowledgeDeleteURL are both POSTed to (v2 API).
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
		knowledgeAddURL:    base + "/knowledge/add/v3",
		knowledgeDeleteURL: base + "/knowledge",
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
	slog.InfoContext(r.Context(), "connect: agent-mcp api hit", "method", r.Method, "path", r.URL.Path)
	var req connectAgentMcpRequest

	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "connect: agent-mcp invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "connect: agent-mcp request", "params", jsonForLog(req))

	agent_mcp, err := h.Queries.InsertAgentMcp(r.Context(), db.InsertAgentMcpParams{
		AgentID: req.AgentId,
		McpID:   req.McpId,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "connect: agent-mcp db insert failed", "agent_id", req.AgentId, "mcp_id", req.McpId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect mcp to agent")
		return
	}

	slog.InfoContext(r.Context(), "connect: agent-mcp success", "status", http.StatusOK, "response", jsonForLog(agent_mcp))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_mcp, nil)
}

func (h *ConnectHandler) DisconnectAgentMcp(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "connect: disconnect agent-mcp api hit", "method", r.Method, "path", r.URL.Path)
	var req connectAgentMcpRequest

	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "connect: disconnect agent-mcp invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "connect: disconnect agent-mcp request", "params", jsonForLog(req))

	rowsAffected, err := h.Queries.SoftDeleteAgentMcp(r.Context(), db.SoftDeleteAgentMcpParams{
		AgentID: req.AgentId,
		McpID:   req.McpId,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "connect: disconnect agent-mcp db soft-delete failed", "agent_id", req.AgentId, "mcp_id", req.McpId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to disconnect mcp from agent")
		return
	}

	if rowsAffected == 0 {
		slog.WarnContext(r.Context(), "connect: disconnect agent-mcp not found", "status", http.StatusNotFound, "params", jsonForLog(req))
		lib.ResponseJSONError(w, http.StatusNotFound, "agent mcp not found")
		return
	}

	slog.InfoContext(r.Context(), "connect: disconnect agent-mcp success", "status", http.StatusNoContent, "rows_affected", rowsAffected)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

type connectAgentKnowledgeRequest struct {
	AgentId     uuid.UUID `json:"agent_id"`
	KnowledgeId uuid.UUID `json:"knowledge_id"`
}

func (h *ConnectHandler) ConnectAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "connect: agent-knowledge api hit", "method", r.Method, "path", r.URL.Path)
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "connect: agent-knowledge invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "connect: agent-knowledge request", "params", jsonForLog(req))

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), req.KnowledgeId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(r.Context(), "connect: agent-knowledge knowledge not found", "status", http.StatusNotFound, "knowledge_id", req.KnowledgeId)
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}
		slog.ErrorContext(r.Context(), "connect: agent-knowledge db select knowledge failed", "knowledge_id", req.KnowledgeId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect knowledge to agent")
		return
	}
	slog.InfoContext(r.Context(), "connect: agent-knowledge selected knowledge", "knowledge_id", req.KnowledgeId, "source_uri", knowledge.SourceUri)

	agent_knowledge, err := h.Queries.InsertAgentKnowledge(r.Context(), db.InsertAgentKnowledgeParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "connect: agent-knowledge db insert failed", "agent_id", req.AgentId, "knowledge_id", req.KnowledgeId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to connect knowledge to agent")
		return
	}
	slog.InfoContext(r.Context(), "connect: agent-knowledge db insert success", "agent_knowledge", jsonForLog(agent_knowledge))

	// The RAG collection to ingest into is the agent's own collection. Best
	// effort: if the agent lookup fails, the rag-service falls back to its
	// default collection when collection_name is empty.
	var milvusCollection string
	if agent, err := h.Queries.SelectAgentById(r.Context(), req.AgentId); err != nil {
		slog.ErrorContext(r.Context(), "connect: agent-knowledge agent lookup for milvus collection failed", "agent_id", req.AgentId, "error", err)
	} else {
		milvusCollection = agent.MilvusCollection
	}

	go h.triggerKnowledgeConversion(agent_knowledge.ID, req.AgentId, req.KnowledgeId, knowledge.SourceUri, knowledge.SourceType, knowledge.IsCrawl, milvusCollection)
	slog.InfoContext(r.Context(), "connect: agent-knowledge rag conversion trigger queued", "agent_knowledge_id", agent_knowledge.ID, "agent_id", req.AgentId, "knowledge_id", req.KnowledgeId, "collection", milvusCollection)

	slog.InfoContext(r.Context(), "connect: agent-knowledge success", "status", http.StatusOK, "response", jsonForLog(agent_knowledge))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent_knowledge, nil)
}

// triggerKnowledgeConversion fires the RAG ingestion webhook in the background.
// Ingestion is legitimately long-running, so this is fire-and-forget: it does
// NOT mark the row 'failed' on a slow or dropped connection. The rag-service
// owns the final 'completed'/'failed' status and reports it via the
// agent-knowledge-status callback endpoint. Only a payload we cannot even build
// (which means the row can never complete) is marked failed here.
func (h *ConnectHandler) triggerKnowledgeConversion(agentKnowledgeID, agentID, knowledgeID uuid.UUID, sourceURI *string, sourceType string, isCrawl bool, milvusCollection string) {
	// Generous ceiling so we don't cancel a long-but-progressing ingestion.
	// The client also has its own 10-minute Timeout as a hard cap.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	payload, err := json.Marshal(knowledgeV2Body(agentKnowledgeID, agentID, knowledgeID, sourceURI, sourceType, isCrawl, milvusCollection))
	if err != nil {
		slog.ErrorContext(ctx, "connect: rag knowledge-add payload marshal failed", "agent_knowledge_id", agentKnowledgeID, "agent_id", agentID, "knowledge_id", knowledgeID, "source_uri", sourceURI, "collection", milvusCollection, "error", err)
		h.markKnowledgeConversionFailed(agentKnowledgeID, err)
		return
	}

	slog.InfoContext(ctx, "connect: rag knowledge-add fetch start", "method", http.MethodPost, "url", h.knowledgeAddURL, "agent_knowledge_id", agentKnowledgeID, "agent_id", agentID, "knowledge_id", knowledgeID, "source_uri", sourceURI, "collection", milvusCollection, "payload", string(payload))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.knowledgeAddURL, bytes.NewReader(payload))
	if err != nil {
		slog.ErrorContext(ctx, "connect: rag knowledge-add request build failed", "url", h.knowledgeAddURL, "payload", string(payload), "error", err)
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
		slog.ErrorContext(ctx, "connect: rag knowledge-add fetch failed (ignored, status owned by rag-service)", "method", http.MethodPost, "url", h.knowledgeAddURL, "duration", duration, "payload", string(payload), "error", err)
		return
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	bodyText := truncateForLog(string(body), 4096)
	if readErr != nil {
		slog.ErrorContext(ctx, "connect: rag knowledge-add response body read failed", "status", resp.StatusCode, "duration", duration, "error", readErr)
	}

	slog.InfoContext(ctx, "connect: rag knowledge-add fetch returned", "method", http.MethodPost, "url", h.knowledgeAddURL, "status", resp.StatusCode, "duration", duration, "response_body", bodyText)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx is logged but not marked failed here either — the
		// rag-service already reports 'failed' via callback on its errors.
		slog.WarnContext(ctx, "connect: rag knowledge-add fetch non-2xx (ignored, status owned by rag-service)", "agent_knowledge_id", agentKnowledgeID, "status", resp.StatusCode, "body", bodyText)
		return
	}

	slog.InfoContext(ctx, "connect: rag knowledge-add fetch success", "agent_knowledge_id", agentKnowledgeID, "status", resp.StatusCode, "body", bodyText)
}

func (h *ConnectHandler) markKnowledgeConversionFailed(agentKnowledgeID uuid.UUID, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.ErrorContext(ctx, "connect: knowledge conversion trigger failed", "agent_knowledge_id", agentKnowledgeID, "error", cause)

	if _, err := h.Queries.UpdateAgentKnowledgeStatus(ctx, db.UpdateAgentKnowledgeStatusParams{
		Status: "failed",
		ID:     agentKnowledgeID,
	}); err != nil {
		slog.ErrorContext(ctx, "connect: mark agent_knowledge as failed failed", "agent_knowledge_id", agentKnowledgeID, "error", err)
	}
}

// RetryAgentKnowledge re-triggers RAG ingestion for a knowledge whose previous
// insertion failed. Keyed on the agent_id + knowledge_id pair (same body as
// connect) so the frontend needs no extra id. Flips status back to 'pending'
// and reuses triggerKnowledgeConversion; the rag-service owns the final status.
func (h *ConnectHandler) RetryAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "connect: retry agent-knowledge api hit", "method", r.Method, "path", r.URL.Path)
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "connect: retry agent-knowledge invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "connect: retry agent-knowledge request", "params", jsonForLog(req))

	ak, err := h.Queries.SelectAgentKnowledgeByPair(r.Context(), db.SelectAgentKnowledgeByPairParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
			return
		}
		slog.ErrorContext(r.Context(), "connect: retry agent-knowledge lookup failed", "agent_id", req.AgentId, "knowledge_id", req.KnowledgeId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to retry knowledge insertion")
		return
	}
	// Only failed insertions are retryable; pending/completed are no-ops.
	if ak.Status != "failed" {
		lib.ResponseJSONError(w, http.StatusConflict, "knowledge insertion is not in a failed state")
		return
	}

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), req.KnowledgeId)
	if err != nil {
		slog.ErrorContext(r.Context(), "connect: retry agent-knowledge knowledge lookup failed", "knowledge_id", req.KnowledgeId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to retry knowledge insertion")
		return
	}

	// Flip back to pending so the UI shows in-flight; conversion drives it on.
	if _, err := h.Queries.UpdateAgentKnowledgeStatus(r.Context(), db.UpdateAgentKnowledgeStatusParams{
		Status: "pending",
		ID:     ak.ID,
	}); err != nil {
		slog.ErrorContext(r.Context(), "connect: retry agent-knowledge status reset failed", "agent_knowledge_id", ak.ID, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to retry knowledge insertion")
		return
	}

	var milvusCollection string
	if agent, err := h.Queries.SelectAgentById(r.Context(), req.AgentId); err != nil {
		slog.ErrorContext(r.Context(), "connect: retry agent-knowledge agent lookup for milvus collection failed", "agent_id", req.AgentId, "error", err)
	} else {
		milvusCollection = agent.MilvusCollection
	}

	go h.triggerKnowledgeConversion(ak.ID, req.AgentId, req.KnowledgeId, knowledge.SourceUri, knowledge.SourceType, knowledge.IsCrawl, milvusCollection)
	slog.InfoContext(r.Context(), "connect: retry agent-knowledge rag conversion trigger queued", "agent_knowledge_id", ak.ID, "agent_id", req.AgentId, "knowledge_id", req.KnowledgeId, "collection", milvusCollection)

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{"status": "pending"}, nil)
}

func (h *ConnectHandler) DisconnectAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "connect: disconnect agent-knowledge api hit", "method", r.Method, "path", r.URL.Path)
	var req connectAgentKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "connect: disconnect agent-knowledge invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "connect: disconnect agent-knowledge request", "params", jsonForLog(req))

	agentKnowledgeID, err := h.Queries.SoftDeleteAgentKnowledgeByPair(r.Context(), db.SoftDeleteAgentKnowledgeByPairParams{
		AgentID:     req.AgentId,
		KnowledgeID: req.KnowledgeId,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(r.Context(), "connect: disconnect agent-knowledge not found", "status", http.StatusNotFound, "params", jsonForLog(req))
			lib.ResponseJSONError(w, http.StatusNotFound, "agent knowledge not found")
			return
		}
		slog.ErrorContext(r.Context(), "connect: disconnect agent-knowledge db soft-delete failed", "agent_id", req.AgentId, "knowledge_id", req.KnowledgeId, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to disconnect knowledge from agent")
		return
	}

	// Look up the knowledge row so the delete body matches connect's exactly
	// (source_uri, source_type, is_crawl). Best effort: zero values on failure.
	var sourceURI *string
	var sourceType string
	var isCrawl bool
	if knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), req.KnowledgeId); err != nil {
		slog.ErrorContext(r.Context(), "connect: disconnect agent-knowledge knowledge lookup failed", "knowledge_id", req.KnowledgeId, "error", err)
	} else {
		sourceURI = knowledge.SourceUri
		sourceType = knowledge.SourceType
		isCrawl = knowledge.IsCrawl
	}

	// Same collection resolution as connect: delete from the agent's own
	// collection. Empty = rag-service falls back to its default collection.
	var milvusCollection string
	if agent, err := h.Queries.SelectAgentById(r.Context(), req.AgentId); err != nil {
		slog.ErrorContext(r.Context(), "connect: disconnect agent-knowledge agent lookup for milvus collection failed", "agent_id", req.AgentId, "error", err)
	} else {
		milvusCollection = agent.MilvusCollection
	}

	go h.triggerKnowledgeDeletion(agentKnowledgeID, req.AgentId, req.KnowledgeId, sourceURI, sourceType, isCrawl, milvusCollection)
	slog.InfoContext(r.Context(), "connect: disconnect agent-knowledge rag deletion trigger queued", "agent_knowledge_id", agentKnowledgeID, "agent_id", req.AgentId, "knowledge_id", req.KnowledgeId, "collection", milvusCollection)

	slog.InfoContext(r.Context(), "connect: disconnect agent-knowledge success", "status", http.StatusNoContent)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

// triggerKnowledgeDeletion fires the knowledge REST API delete endpoint in the
// background. Failures are logged only; the local disconnect has already
// succeeded by the time this runs.
func (h *ConnectHandler) triggerKnowledgeDeletion(agentKnowledgeID, agentID, knowledgeID uuid.UUID, sourceURI *string, sourceType string, isCrawl bool, milvusCollection string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete is DELETE with no body: everything goes in the query string.
	// v2SourceType := "file"
	// if sourceType == "web" {
	// 	v2SourceType = "web"
	// }
	q := neturl.Values{}
	q.Set("agent_knowledge_id", agentKnowledgeID.String())
	q.Set("document_id", knowledgeID.String())
	// if sourceURI != nil {
	// 	q.Set("document_link", *sourceURI)
	// }
	// q.Set("source_type", v2SourceType)
	// if v2SourceType == "web" && isCrawl {
	// 	q.Set("scrape_mode", "crawl")
	// }
	// q.Set("agent_id", agentID.String())
	q.Set("collection_name", milvusCollection)
	url := h.knowledgeDeleteURL + "?" + q.Encode()
	slog.InfoContext(ctx, "connect: rag knowledge-delete fetch start", "method", http.MethodDelete, "url", url, "knowledge_id", knowledgeID, "collection", milvusCollection)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		slog.ErrorContext(ctx, "connect: rag knowledge-delete request build failed", "knowledge_id", knowledgeID, "url", url, "error", err)
		return
	}

	started := time.Now()
	resp, err := h.HTTPClient.Do(req)
	duration := time.Since(started)
	if err != nil {
		slog.ErrorContext(ctx, "connect: rag knowledge-delete fetch failed", "method", http.MethodDelete, "url", url, "duration", duration, "knowledge_id", knowledgeID, "error", err)
		return
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	bodyText := truncateForLog(string(body), 4096)
	if readErr != nil {
		slog.ErrorContext(ctx, "connect: rag knowledge-delete response body read failed", "status", resp.StatusCode, "duration", duration, "error", readErr)
	}

	slog.InfoContext(ctx, "connect: rag knowledge-delete fetch returned", "method", http.MethodDelete, "url", url, "status", resp.StatusCode, "duration", duration, "response_body", bodyText)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.WarnContext(ctx, "connect: rag knowledge-delete fetch non-2xx", "knowledge_id", knowledgeID, "status", resp.StatusCode, "body", bodyText)
		return
	}

	slog.InfoContext(ctx, "connect: rag knowledge-delete fetch success", "knowledge_id", knowledgeID, "status", resp.StatusCode, "body", bodyText)
}

// knowledgeV2Body builds the shared /knowledge/{add,delete}/v2 request body.
// scrape_mode is only sent for web sources: "crawl" when is_crawl, else
// "single". source_type passes through except non-web (file) sources report
// "file" to the v2 API regardless of the stored extension.
func knowledgeV2Body(agentKnowledgeID, agentID, knowledgeID uuid.UUID, sourceURI *string, sourceType string, isCrawl bool, milvusCollection string) map[string]any {
	v2SourceType := "file"
	if sourceType == "web" {
		v2SourceType = "web"
	}

	body := map[string]any{
		"agent_knowledge_id": agentKnowledgeID,
		"document_id":        knowledgeID,
		"document_link":      sourceURI,
		"source_type":        v2SourceType,
		"agent_id":           agentID,
		"collection_name":    milvusCollection,
	}
	if v2SourceType == "web" && isCrawl {
		body["scrape_mode"] = "crawl"
	}
	return body
}
