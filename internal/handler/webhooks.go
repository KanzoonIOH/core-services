package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var insecureWebhookHosts = map[string]bool{
	"shared-n8n-d7d093-217-216-35-206.sslip.io": true,
	"103.67.43.198": true,
}

type WebhookHandler struct {
	HTTPClient *http.Client
	Queries    db.Querier
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

func NewWebhookHandler(conn *pgxpool.Pool) *WebhookHandler {
	return &WebhookHandler{
		Queries: db.New(conn),
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

	if !agent.IsActive {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	targetURL := agent.WebhookUri
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("chat webhook proxy working: target=%s", targetURL)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		log.Printf("error dsini")
		http.Error(w, "failed to create webhook request", http.StatusInternalServerError)
		return
	}
	copyForwardHeaders(req.Header, r.Header)

	res, err := h.HTTPClient.Do(req)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to forward webhook request", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()

	copyResponseHeaders(w.Header(), res.Header)
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
