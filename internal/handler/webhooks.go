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
	"log"
	"net"
	"net/http"
	"net/netip"
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

	log.Printf("chat webhook proxy: agent=%s session=%s target=%s", id, conversationID, targetURL)
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
	h.storeMessage(conversationID, db.MessageRoleUser, bodyMap["chatInput"], bodyMap["attachments"], nil)

	if agent.WebhookInputField != "" && agent.WebhookInputField != "chatInput" {
		if v, ok := bodyMap["chatInput"]; ok {
			bodyMap[agent.WebhookInputField] = v
			delete(bodyMap, "chatInput")
		}
	}

	bodyMap["sessionId"] = conversationID
	bodyMap["agent_id"] = id.String()
	bodyMap["tone"] = string(agent.Tone)
	bodyMap["length"] = string(agent.ResponseLength)
	bodyMap["style"] = string(agent.CommunicationStyle)

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

	log.Printf("chat webhook request: %s", curlPreview(req, modifiedBody))

	res, err := h.HTTPClient.Do(req)
	if err != nil {
		log.Printf("chat webhook forward error: %v", err)
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
			outputField := agent.WebhookOutputField
			if outputField == "" {
				outputField = "output"
			}
			h.storeMessage(conversationID, db.MessageRoleAssistant, respMap[outputField], respMap["attachments"], respMap["data"])
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
			log.Printf("store message conv=%s role=%s: %v", conversationID, role, err)
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
		log.Printf("kafka publish %s: %v", TopicConversationActivity, err)
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
		log.Printf("kafka publish %s: %v", TopicChatMessage, err)
	}
	log.Printf("kafka publish %s: published", TopicChatMessage)
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
