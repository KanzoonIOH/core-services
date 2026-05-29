package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/app/middleware"
	"aiac-service/internal/lib"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Topic names
const (
	TopicChatMessage     = "chat.webhook.message"
	TopicConversationEnd = "chat.conversation.end"
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
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	if middleware.IsApiKeyAuth(r.Context()) && !agent.IsActive {
		lib.ResponseJSONError(w, http.StatusForbidden, "agent is not active")
		return
	}

	// Resolve conversation_id: use caller-supplied sessionId or generate a new one.
	conversationID := r.URL.Query().Get("sessionId")
	if conversationID == "" {
		conversationID = uuid.New().String()
	}

	targetURL := agent.WebhookUri
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("chat webhook proxy: agent=%s session=%s target=%s", id, conversationID, targetURL)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create webhook request")
		return
	}
	copyForwardHeaders(req.Header, r.Header)

	hitTime := time.Now().UTC()

	res, err := h.HTTPClient.Do(req)
	if err != nil {
		log.Printf("chat webhook forward error: %v", err)
		lib.ResponseJSONError(w, http.StatusBadGateway, "failed to forward webhook request")
		return
	}
	defer res.Body.Close()

	responseTimeMs := time.Since(hitTime).Milliseconds()
	isSuccess := res.StatusCode >= 200 && res.StatusCode < 300

	// Publish a single message event with full metrics.
	if err := h.Kafka.Publish(context.Background(), TopicChatMessage, map[string]any{
		"agent_id":         id.String(),
		"conversation_id":  conversationID,
		"status_code":      res.StatusCode,
		"response_time_ms": responseTimeMs,
		"is_success":       isSuccess,
		"occurred_at":      hitTime,
	}); err != nil {
		log.Printf("kafka publish %s: %v", TopicChatMessage, err)
	}

	// Echo the conversation_id back so the caller can reuse it on subsequent messages.
	copyResponseHeaders(w.Header(), res.Header)
	w.Header().Set("X-Session-Id", conversationID)
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
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
