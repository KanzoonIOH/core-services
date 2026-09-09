package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Topic names
const (
	TopicChatMessage           = "chat.webhook.message"
	TopicConversationActivity  = "chat.conversation.activity"
	TopicConversationEnd       = "chat.conversation.end"
	TopicConversationAnalytics = "chat.conversation.analytics"
)

var insecureWebhookHosts = map[string]bool{
	"shared-n8n-d7d093-217-216-35-206.sslip.io": true,
	"103.67.43.198": true,
}

type WebhookHandler struct {
	HTTPClient *http.Client
	Queries    db.Querier
	Kafka      *lib.KafkaProducer
}

// pluckField resolves an output-field path against a decoded JSON body.
// A plain key ("output") is the common case and matches first, so keys that
// happen to contain dots still work. Otherwise the path is walked segment by
// segment with optional array indices: "choices[0].message.content".
// Returns nil when any segment is missing or the wrong shape.
func pluckField(v any, path string) any {
	if m, ok := v.(map[string]any); ok {
		if hit, ok := m[path]; ok {
			return hit
		}
	}
	for _, seg := range strings.Split(path, ".") {
		key, idx, _ := strings.Cut(seg, "[")
		if key != "" {
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[key]
		}
		// idx holds the remaining "0]" / "0][1]" indices, if any.
		for idx != "" {
			var num string
			num, idx, _ = strings.Cut(idx, "]")
			i, err := strconv.Atoi(num)
			arr, ok := v.([]any)
			if err != nil || !ok || i < 0 || i >= len(arr) {
				return nil
			}
			v = arr[i]
			idx = strings.TrimPrefix(idx, "[")
		}
	}
	return v
}

// clientIP returns the caller's source IP. It trusts X-Forwarded-For (first
// hop) when present, since this service runs behind a proxy/load balancer.
// ponytail: trusts XFF unconditionally; tighten to trusted-proxy-only if the
// service is ever exposed directly to untrusted clients.
func clientIP(r *http.Request) netip.Addr {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if addr, err := netip.ParseAddr(first); err == nil {
			return addr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, _ := netip.ParseAddr(host)
	return addr
}

func ipAllowed(ip netip.Addr, allowed []netip.Addr) bool {
	for _, a := range allowed {
		if a == ip {
			return true
		}
	}
	return false
}

// originAllowed does exact-string matching, same as the global CORS middleware.
// ponytail: exact match only; add *.example.com wildcards if a widget needs them.
func originAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}

type hostTLSBypassTransport struct {
	secure   http.RoundTripper
	insecure http.RoundTripper
}

func (t *hostTLSBypassTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if insecureWebhookHosts[req.URL.Hostname()] {
		return t.insecure.RoundTrip(req)
	}
	return t.secure.RoundTrip(req)
}

func NewWebhookHandler(conn *pgxpool.Pool, kafka *lib.KafkaProducer) *WebhookHandler {
	return &WebhookHandler{
		Queries: db.New(conn),
		Kafka:   kafka,
		HTTPClient: &http.Client{
			Timeout: 2 * time.Minute,
			Transport: &hostTLSBypassTransport{
				secure: http.DefaultTransport,
				insecure: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
		},
	}
}

func (h *WebhookHandler) ForwardChatWebhook(w http.ResponseWriter, r *http.Request) {
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

	// id may be an orchestrator, not an agent. Try agent first; on NotFound,
	// fall through to the orchestrator chat forwarder.
	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.forwardOrchestratorChat(w, r, id, conversationID, hitTime, false)
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

	// Empty list = allow all (backward compatible). Otherwise the caller's IP
	// must be in the agent's whitelist.
	if len(agent.WebhookAllowedIps) > 0 && !ipAllowed(clientIP(r), agent.WebhookAllowedIps) {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusForbidden, hitTime, "caller ip not allowed")
		lib.ResponseJSONError(w, http.StatusForbidden, "caller ip not allowed")
		return
	}

	// Browser request (Origin present): must pass the per-agent origin allowlist,
	// and we reflect ACAO so the browser accepts the response. Server-to-server
	// calls carry no Origin and are governed by the IP whitelist above.
	if origin := r.Header.Get("Origin"); origin != "" {
		if len(agent.WebhookAllowedOrigins) > 0 && !originAllowed(origin, agent.WebhookAllowedOrigins) {
			h.publishWebhookMessage(id.String(), conversationID, http.StatusForbidden, hitTime, "origin not allowed")
			lib.ResponseJSONError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}

	targetURL := agent.WebhookUri
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	slog.InfoContext(r.Context(), "chat webhook proxy", "agent", id, "session", conversationID, "target", targetURL)
	h.publishConversationActivity(id.String(), conversationID, hitTime)

	// Read body and inject sessionId before forwarding.
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
	// Callers always send the message under "chatInput". Map it to the agent's
	// configured input field (e.g. "query") before forwarding, so the upstream
	// agent receives the field name it expects. Persist before the rename so the
	// stored value is the same either way.
	h.ensureConversation(r.Context(), id, conversationID)
	h.storeMessage(conversationID, db.MessageRoleUser, bodyMap["chatInput"], storedAttachments(bodyMap), nil)

	if agent.WebhookInputField != "" && agent.WebhookInputField != "chatInput" {
		if v, ok := bodyMap["chatInput"]; ok {
			bodyMap[agent.WebhookInputField] = v
			delete(bodyMap, "chatInput")
		}
	}

	bodyMap["sessionId"] = conversationID
	bodyMap["agentId"] = id.String()
	if agent.PersonaEnabled {
		bodyMap["tone"] = string(agent.Tone)
		bodyMap["length"] = string(agent.ResponseLength)
		bodyMap["style"] = string(agent.CommunicationStyle)
	}

	// App-wide system prompt applied to every agent. Best-effort: a config read
	// failure must not block the chat, so fall back to empty values.
	if agent.GuardrailEnabled {
		cfg, cfgErr := h.Queries.GetGlobalConfig(r.Context())
		if cfgErr != nil {
			slog.ErrorContext(r.Context(), "global config read failed", "error", cfgErr)
		}
		bodyMap["systemPrompt"] = map[string]any{
			"agent_name":           cfg.AgentName,
			"industry_description": cfg.IndustryDescription,
			"guardrail":            cfg.Guardrail,
		}
	}

	// Reserved "headers" object in the incoming body carries per-request
	// dynamic header values. Pull it out so it isn't forwarded in the body.
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

	// Inject the agent's configured body fields. Static fields set a fixed
	// value; dynamic fields must be supplied by the caller (enforced).
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
			// keep the caller-supplied value as-is
		} else {
			bodyMap[f.Key] = f.Value
		}
	}

	// Resolve the agent's configured auth/header fields. Static uses the fixed
	// value; dynamic must be supplied by the caller under body.headers
	// (enforced). Open agent = no fields = no headers added.
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
	// Agent-configured auth/headers override forwarded ones.
	for k, v := range outHeaders {
		req.Header.Set(k, v)
	}

	slog.InfoContext(r.Context(), "chat webhook request", "curl", curlPreview(req, modifiedBody))

	res, err := h.HTTPClient.Do(req)
	if err != nil {
		slog.ErrorContext(r.Context(), "chat webhook forward error", "error", err)
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

	// Buffer the response so we can persist the AI reply, then relay it verbatim.
	// The frontend reads the agent's configured webhook_output_field to know
	// which key holds the reply text — no remapping needed here.
	respBytes, _ := io.ReadAll(res.Body)
	if isSuccess {
		var respMap map[string]any
		if json.Unmarshal(respBytes, &respMap) == nil {
			// Must match the frontend/API default (chatWithAgent: "reply").
			// If these disagree, the assistant reply is stored as nil and the
			// turn is silently dropped from history.
			outputField := agent.WebhookOutputField
			if outputField == "" {
				outputField = "reply"
			}
			h.storeMessage(conversationID, db.MessageRoleAssistant, pluckField(respMap, outputField), respMap["attachments"], respMap["data"])
		}
	}

	// Echo the conversation_id back so the caller can reuse it on subsequent messages.
	copyResponseHeaders(w.Header(), res.Header)
	w.Header().Set("X-Session-Id", conversationID)
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(respBytes)
}

// PreflightChatWebhook answers the browser's CORS preflight (OPTIONS) for a
// specific agent. No auth, no body. Reflects ACAO when the Origin is allowed by
// the agent (or the agent has no restriction).
func (h *WebhookHandler) PreflightChatWebhook(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || origin == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err == nil && (len(agent.WebhookAllowedOrigins) == 0 || originAllowed(origin, agent.WebhookAllowedOrigins)) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Session-Id")
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureConversation upserts the conversations row for this session so chat
// history can be listed per conversation. Best-effort and off the request
// path, same contract as storeMessage. Idempotent (ON CONFLICT DO NOTHING).
// The owner is taken from the JWT claims when present; API-key traffic has no
// user behind it and stores a NULL user_id.
func (h *WebhookHandler) ensureConversation(ctx context.Context, agentID uuid.UUID, conversationID string) {
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return
	}
	var userID *uuid.UUID
	if claims, ok := middleware.ClaimsFromContext(ctx); ok {
		userID = &claims.UserID
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Queries.UpsertConversation(ctx, db.UpsertConversationParams{
			ID:      convID,
			AgentID: agentID,
			UserID:  userID,
		}); err != nil {
			slog.ErrorContext(ctx, "upsert conversation", "conversation", conversationID, "error", err)
		}
	}()
}

// storeMessage persists one chat message to Postgres. Best-effort and off the
// request path: history is durable truth, but a failure here must never break
// the live proxy (which Redis already backs). text, attachments and data are
// all optional — a chat turn has text, an upload-only turn has attachments,
// a REPORT reply has data. Skips a turn that carries none of them.
func (h *WebhookHandler) storeMessage(conversationID string, role db.MessageRole, text any, attachments any, data any) {
	var content *string
	if s, ok := text.(string); ok && s != "" {
		content = &s
	}
	// attachments defaults to [] (column is NOT NULL); data stays NULL when absent.
	attachJSON := jsonOrDefault(attachments, []byte("[]"))
	dataJSON := jsonOrDefault(data, nil)

	if content == nil && len(dataJSON) == 0 && string(attachJSON) == "[]" {
		return
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Queries.InsertMessage(ctx, db.InsertMessageParams{
			ConversationID: convID,
			Role:           role,
			Content:        content,
			Attachments:    attachJSON,
			Data:           dataJSON,
		}); err != nil {
			slog.ErrorContext(ctx, "store message", "conversation", conversationID, "role", role, "error", err)
		}
	}()
}

// jsonOrDefault marshals v to JSON, returning def if v is nil or fails to marshal.
func jsonOrDefault(v any, def []byte) json.RawMessage {
	if v == nil {
		return def
	}
	b, err := json.Marshal(v)
	if err != nil {
		return def
	}
	return b
}

func (h *WebhookHandler) publishConversationActivity(agentID, conversationID string, occurredAt time.Time) {
	if h.Kafka == nil {
		return
	}

	if err := h.Kafka.Publish(context.Background(), TopicConversationActivity, map[string]any{
		"agent_id":        agentID,
		"conversation_id": conversationID,
		"occurred_at":     occurredAt,
	}); err != nil {
		slog.Error("kafka publish", "topic", TopicConversationActivity, "error", err)
	} else {
		// ponytail: diagnostic — confirm activity publish; remove once stable
		slog.Info("published activity", "agent", agentID, "conversation", conversationID)
	}
}

func (h *WebhookHandler) publishWebhookMessage(agentID, conversationID string, statusCode int, hitTime time.Time, errorMessage string) {
	if h.Kafka == nil {
		return
	}

	responseTimeMs := time.Since(hitTime).Milliseconds()
	isSuccess := statusCode >= 200 && statusCode < 300 && errorMessage == ""

	// Publish a single message event with full metrics.
	if err := h.Kafka.Publish(context.Background(), TopicChatMessage, map[string]any{
		"agent_id":         agentID,
		"conversation_id":  conversationID,
		"status_code":      statusCode,
		"response_time_ms": responseTimeMs,
		"is_success":       isSuccess,
		"error":            errorMessage,
		"occurred_at":      hitTime,
	}); err != nil {
		slog.Error("kafka publish", "topic", TopicChatMessage, "error", err)
	}
	slog.Info("kafka publish", "topic", TopicChatMessage, "status", "published")
}

// curlPreview reconstructs the outgoing request as a copy-pasteable curl line
// for debugging. Authorization is redacted so tokens don't leak into logs.
func curlPreview(req *http.Request, body []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "curl --request %s --url %s", req.Method, req.URL.String())
	for k, vals := range req.Header {
		for _, v := range vals {
			if strings.EqualFold(k, "Authorization") {
				v = "[redacted]"
			}
			fmt.Fprintf(&b, " --header '%s: %s'", k, v)
		}
	}
	if len(body) > 0 {
		fmt.Fprintf(&b, " --data '%s'", body)
	}
	return b.String()
}

func copyForwardHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) || strings.EqualFold(key, "Host") || strings.EqualFold(key, "Authorization") {
			continue
		}
		// Drop the caller's Accept-Encoding (browsers send "gzip, deflate, br,
		// zstd"). Go's transport only decompresses gzip it asked for itself, so
		// forwarding this hands us a br/zstd body we can't parse — the reply
		// still reaches the browser, but the assistant turn silently fails to
		// persist and the history is empty on reload.
		if strings.EqualFold(key, "Accept-Encoding") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) || isCorsHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isCorsHeader(key string) bool {
	return strings.HasPrefix(strings.ToLower(key), "access-control-")
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

// forwardOrchestratorChat handles /chat/{id} when the id resolves to an
// orchestrator instead of an agent. It reads the orchestrator's webhook_uri
// and forwards the request there, same body shape as agent chat. Orchestrators
// have no body/header field injection, no IP/origin allowlist, and no
// input/output field mapping — the body is assembled minimally.
func (h *WebhookHandler) forwardOrchestratorChat(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	conversationID string,
	hitTime time.Time,
	stream bool,
) {
	orch, err := h.Queries.SelectOrchestratorById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.publishWebhookMessage(id.String(), conversationID, http.StatusNotFound, hitTime, "agent not found")
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		h.publishWebhookMessage(id.String(), conversationID, http.StatusInternalServerError, hitTime, "failed to get orchestrator")
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get orchestrator")
		return
	}

	if middleware.IsApiKeyAuth(r.Context()) && !orch.IsActive {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusForbidden, hitTime, "orchestrator is not active")
		lib.ResponseJSONError(w, http.StatusForbidden, "orchestrator is not active")
		return
	}

	if orch.WebhookUri == "" {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, "orchestrator has no webhook_uri configured")
		lib.ResponseJSONError(w, http.StatusBadRequest, "orchestrator has no webhook_uri configured")
		return
	}

	// Same fallback as agents: an upstream without a /stream sibling is served
	// from the plain endpoint, and the client's SSE reader handles the single
	// JSON body it gets back.
	stream = stream && orch.WebhookStreamEnabled

	targetURL := orch.WebhookUri
	if stream {
		targetURL = strings.TrimRight(orch.WebhookUri, "/") + "/stream"
	}
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	slog.InfoContext(r.Context(), "chat orchestrator proxy", "id", id, "session", conversationID, "target", targetURL, "stream", stream)
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

	h.ensureConversation(r.Context(), id, conversationID)
	h.storeMessage(conversationID, db.MessageRoleUser, bodyMap["chatInput"], storedAttachments(bodyMap), nil)

	// Same input mapping as agents: callers always send "chatInput", the
	// orchestrator may expect a different key upstream.
	if orch.WebhookInputField != "" && orch.WebhookInputField != "chatInput" {
		if v, ok := bodyMap["chatInput"]; ok {
			bodyMap[orch.WebhookInputField] = v
			delete(bodyMap, "chatInput")
		}
	}

	// Reserved "headers" object carries per-request dynamic header values.
	callerHeaders := popCallerHeaders(bodyMap)

	bodyMap["sessionId"] = conversationID
	bodyMap["agentId"] = id.String()

	// Parse the orchestrator's persona blob for tone/length/style. Falls back
	// to defaults if the blob is empty or not JSON.
	if orch.PersonaEnabled {
		tone, length, style := orchestratorPersonaFields(orch.Persona)
		bodyMap["tone"] = tone
		bodyMap["length"] = length
		bodyMap["style"] = style
	}

	if orch.GuardrailEnabled {
		cfg, cfgErr := h.Queries.GetGlobalConfig(r.Context())
		if cfgErr != nil {
			slog.ErrorContext(r.Context(), "global config read failed", "error", cfgErr)
		}
		bodyMap["systemPrompt"] = map[string]any{
			"agent_name":           cfg.AgentName,
			"industry_description": cfg.IndustryDescription,
			"guardrail":            cfg.Guardrail,
		}
		// Merge the orchestrator's own guardrail on top of the global one.
		if orch.Guardrail != "" {
			bodyMap["guardrail"] = orch.Guardrail
		}
	}
	if orch.RoutingGuide != "" {
		bodyMap["routingGuide"] = orch.RoutingGuide
	}

	if msg := applyWebhookBodyFields(bodyMap, orch.WebhookBodyFields); msg != "" {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, msg)
		lib.ResponseJSONError(w, http.StatusBadRequest, msg)
		return
	}
	outHeaders, msg := resolveWebhookHeaders(orch.WebhookHeaderFields, callerHeaders)
	if msg != "" {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusBadRequest, hitTime, msg)
		lib.ResponseJSONError(w, http.StatusBadRequest, msg)
		return
	}

	outputField := orch.WebhookOutputField
	if outputField == "" {
		outputField = "reply"
	}

	modifiedBody, err := json.Marshal(bodyMap)
	if err != nil {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusInternalServerError, hitTime, "failed to encode request body")
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to encode request body")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		h.publishWebhookMessage(id.String(), conversationID, http.StatusInternalServerError, hitTime, "failed to create webhook request")
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create webhook request")
		return
	}
	copyForwardHeaders(req.Header, r.Header)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	// Orchestrator-configured auth/headers override forwarded ones.
	for k, v := range outHeaders {
		req.Header.Set(k, v)
	}

	slog.InfoContext(r.Context(), "chat orchestrator request", "curl", curlPreview(req, modifiedBody))

	res, err := h.HTTPClient.Do(req)
	if err != nil {
		slog.ErrorContext(r.Context(), "chat orchestrator forward error", "error", err)
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

	if stream {
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

		var captured bytes.Buffer
		buf := make([]byte, 32*1024)
		for {
			n, readErr := res.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				captured.Write(buf[:n])
				flusher.Flush()
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				break
			}
		}
		if isSuccess {
			h.persistStreamedReply(conversationID, outputField, captured.Bytes())
		}
		return
	}

	// Non-stream: buffer + relay verbatim, persist the reply.
	respBytes, _ := io.ReadAll(res.Body)
	if isSuccess {
		var respMap map[string]any
		if json.Unmarshal(respBytes, &respMap) == nil {
			h.storeMessage(conversationID, db.MessageRoleAssistant, pluckField(respMap, outputField), respMap["attachments"], respMap["data"])
		}
	}

	copyResponseHeaders(w.Header(), res.Header)
	w.Header().Set("X-Session-Id", conversationID)
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(respBytes)
}

// orchestratorPersonaFields parses the orchestrator persona blob (JSON with
// tone/response_length/communication_style) and returns the values. Falls back
// to defaults when the blob is empty or unparseable.
func orchestratorPersonaFields(raw string) (tone, length, style string) {
	tone = "FRIENDLY"
	length = "MEDIUM"
	style = "EXPERT_ADVISOR"
	if raw == "" {
		return
	}
	var p struct {
		Tone               string `json:"tone"`
		ResponseLength     string `json:"response_length"`
		CommunicationStyle string `json:"communication_style"`
	}
	if json.Unmarshal([]byte(raw), &p) == nil {
		if p.Tone != "" {
			tone = p.Tone
		}
		if p.ResponseLength != "" {
			length = p.ResponseLength
		}
		if p.CommunicationStyle != "" {
			style = p.CommunicationStyle
		}
	}
	return
}
