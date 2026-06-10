package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/app/middleware"
	"aiac-service/internal/lib"
	"bytes"
	"context"
	"crypto/tls"
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
	"github.com/jackc/pgx/v5/pgxpool"
)

// Topic names
const (
	TopicChatMessage             = "chat.webhook.message"
	TopicConversationActivity    = "chat.conversation.activity"
	TopicConversationEnd         = "chat.conversation.end"
	TopicConversationAnalytics   = "chat.conversation.analytics"
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
	bodyMap["sessionId"] = conversationID
	bodyMap["tone"] = string(agent.Tone)
	bodyMap["length"] = string(agent.ResponseLength)
	bodyMap["style"] = string(agent.CommunicationStyle)

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

	// Echo the conversation_id back so the caller can reuse it on subsequent messages.
	copyResponseHeaders(w.Header(), res.Header)
	w.Header().Set("X-Session-Id", conversationID)
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
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
