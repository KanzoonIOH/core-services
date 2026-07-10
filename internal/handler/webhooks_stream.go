package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ForwardChatWebhookStream is a streaming variant of ForwardChatWebhook. It
// relays the upstream response to the client chunk-by-chunk (flushing as data
// arrives, e.g. for SSE / chunked LLM output) instead of buffering the whole
// body. The upstream reply is teed into a buffer so the assistant turn is still
// persisted to Postgres once the stream finishes.
//
// The non-streaming handler is left untouched; this shares only package-level
// helpers. Register at a separate route (e.g. POST /chat/{id}/stream).
func (h *WebhookHandler) ForwardChatWebhookStream(w http.ResponseWriter, r *http.Request) {
	hitTime := time.Now().UTC()
	agentID := chi.URLParam(r, "id")
	conversationID := r.Header.Get("x-session-id")
	if conversationID == "" {
		conversationID = uuid.New().String()
	}

	id, err := uuid.Parse(agentID)
	if err != nil {
		h.publishWebhookMessage(agentID, conversationID, http.StatusBadRequest, hitTime, "invalid id")
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.publishWebhookMessage(id.String(), conversationID, http.StatusNotFound, hitTime, "agent not found")
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		h.publishWebhookMessage(id.String(), conversationID, http.StatusInternalServerError, hitTime, "failed to get agent")
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	if middleware.IsApiKeyAuth(r.Context()) && !agent.IsActive {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusForbidden, hitTime, "agent is not active")
		lib.ResponseJSONError(w, http.StatusForbidden, "agent is not active")
		return
	}

	if len(agent.WebhookAllowedIps) > 0 && !ipAllowed(clientIP(r), agent.WebhookAllowedIps) {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusForbidden, hitTime, "caller ip not allowed")
		lib.ResponseJSONError(w, http.StatusForbidden, "caller ip not allowed")
		return
	}

	if origin := r.Header.Get("Origin"); origin != "" {
		if len(agent.WebhookAllowedOrigins) > 0 && !originAllowed(origin, agent.WebhookAllowedOrigins) {
			h.publishWebhookMessage(id.String(), conversationID, http.StatusForbidden, hitTime, "origin not allowed")
			lib.ResponseJSONError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}

	// The streaming endpoint forwards to the upstream's /stream variant:
	// webhook_uri + "/stream" (trailing slash on the configured URI is trimmed
	// so we don't produce "//stream").
	targetURL := strings.TrimRight(agent.WebhookUri, "/") + "/stream"
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("chat webhook stream proxy: agent=%s session=%s target=%s", id, conversationID, targetURL)
	h.publishConversationActivity(id.String(), conversationID, hitTime)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, "failed to read request body")
		lib.ResponseJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var bodyMap map[string]any
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
			h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, "invalid JSON body")
			lib.ResponseJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	} else {
		bodyMap = map[string]any{}
	}

	h.ensureConversation(id, conversationID)
	h.storeMessage(conversationID, db.MessageRoleUser, bodyMap["chatInput"], bodyMap["attachments"], nil)

	if agent.WebhookInputField != "" && agent.WebhookInputField != "chatInput" {
		if v, ok := bodyMap["chatInput"]; ok {
			bodyMap[agent.WebhookInputField] = v
			delete(bodyMap, "chatInput")
		}
	}

	bodyMap["sessionId"] = conversationID
	bodyMap["agentId"] = id.String()
	bodyMap["tone"] = string(agent.Tone)
	bodyMap["length"] = string(agent.ResponseLength)
	bodyMap["style"] = string(agent.CommunicationStyle)

	cfg, cfgErr := h.Queries.GetGlobalConfig(r.Context())
	if cfgErr != nil {
		log.Printf("[core-service][chat-webhook-stream] global config read failed: %v", cfgErr)
	}
	bodyMap["systemPrompt"] = map[string]any{
		"agent_name":           cfg.AgentName,
		"industry_description": cfg.IndustryDescription,
		"guardrail":            cfg.Guardrail,
	}

	callerHeaders := map[string]string{}
	if raw, ok := bodyMap["headers"]; ok {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					callerHeaders[k] = s
				}
			}
		}
		delete(bodyMap, "headers")
	}

	for _, f := range parseWebhookFields(agent.WebhookBodyFields) {
		if f.Key == "" {
			continue
		}
		if f.Type == "dynamic" {
			v, ok := bodyMap[f.Key]
			if !ok || v == nil || v == "" {
				msg := fmt.Sprintf("missing required field %q", f.Key)
				h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, msg)
				lib.ResponseJSONError(w, http.StatusBadRequest, msg)
				return
			}
		} else {
			bodyMap[f.Key] = f.Value
		}
	}

	outHeaders := map[string]string{}
	for _, f := range parseWebhookFields(agent.WebhookHeaderFields) {
		if f.Key == "" {
			continue
		}
		if f.Type == "dynamic" {
			v := callerHeaders[f.Key]
			if v == "" {
				msg := fmt.Sprintf("missing required header %q", f.Key)
				h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, msg)
				lib.ResponseJSONError(w, http.StatusBadRequest, msg)
				return
			}
			outHeaders[f.Key] = v
		} else {
			outHeaders[f.Key] = f.Value
		}
	}

	modifiedBody, err := json.Marshal(bodyMap)
	if err != nil {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusInternalServerError, hitTime, "failed to encode request body")
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to encode request body")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusInternalServerError, hitTime, "failed to create webhook request")
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create webhook request")
		return
	}
	copyForwardHeaders(req.Header, r.Header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range outHeaders {
		req.Header.Set(k, v)
	}

	log.Printf("chat webhook stream request: %s", curlPreview(req, modifiedBody))

	res, err := h.HTTPClient.Do(req)
	if err != nil {
		log.Printf("chat webhook stream forward error: %v", err)
		h.publishWebhookMessage(id.String(), conversationID, http.StatusBadGateway, hitTime, err.Error())
		lib.ResponseJSONError(w, http.StatusBadGateway, "failed to forward webhook request")
		return
	}
	defer res.Body.Close()

	isSuccess := res.StatusCode >= 200 && res.StatusCode < 300
	var errorMessage string
	if !isSuccess {
		errorMessage = fmt.Sprintf("upstream returned status %d", res.StatusCode)
	}
	h.publishWebhookMessage(id.String(), conversationID, res.StatusCode, hitTime, errorMessage)

	// ponytail: needs http.Flusher; without it we cannot stream, so fail loud
	// rather than silently buffering (the non-stream handler already does that).
	flusher, ok := w.(http.Flusher)
	if !ok {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	copyResponseHeaders(w.Header(), res.Header)
	w.Header().Set("X-Session-Id", conversationID)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(res.StatusCode)
	flusher.Flush()

	// Relay chunks to the client as they arrive; tee into captured so the final
	// assistant reply can be persisted. 32KB matches io.Copy's default.
	var captured bytes.Buffer
	buf := make([]byte, 32*1024)
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				log.Printf("chat webhook stream client write error: %v", werr)
				return
			}
			captured.Write(buf[:n])
			flusher.Flush()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			log.Printf("chat webhook stream upstream read error: %v", readErr)
			break
		}
	}

	if isSuccess {
		h.persistStreamedReply(conversationID, agent.WebhookOutputField, captured.Bytes())
	}
}

// persistStreamedReply extracts the assistant reply from a streamed upstream
// response and stores it. It first tries to parse the whole capture as one JSON
// object (many "streaming" webhooks still send a single JSON body); if that
// fails it falls back to concatenating SSE `data:` payload text. Best-effort —
// history persistence must never affect the already-delivered stream.
func (h *WebhookHandler) persistStreamedReply(conversationID, outputField string, raw []byte) {
	if outputField == "" {
		outputField = "reply"
	}

	var respMap map[string]any
	if json.Unmarshal(bytes.TrimSpace(raw), &respMap) == nil {
		h.storeMessage(conversationID, db.MessageRoleAssistant, respMap[outputField], respMap["attachments"], respMap["data"])
		return
	}

	text := extractSSEText(raw)
	if text == "" {
		return
	}
	h.storeMessage(conversationID, db.MessageRoleAssistant, text, nil, nil)
}

// extractSSEText concatenates the text of SSE `data:` lines. For each data line
// it tries JSON first (common shapes: {"delta":"..."} / {"text":"..."} /
// {"content":"..."}); if not JSON, the raw line value is appended. `[DONE]`
// sentinels are ignored.
func extractSSEText(raw []byte) string {
	var out bytes.Buffer
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var m map[string]any
		if json.Unmarshal(payload, &m) == nil {
			for _, k := range []string{"delta", "text", "content", "reply"} {
				if s, ok := m[k].(string); ok {
					out.WriteString(s)
					break
				}
			}
			continue
		}
		out.Write(payload)
	}
	return out.String()
}
