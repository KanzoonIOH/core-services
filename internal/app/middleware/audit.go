package middleware

import (
	"aic3-service/internal/lib"
	"context"
	"net/http"
	"strings"
	"time"
)

// TopicAudit is the Kafka topic every write/delete audit event is published to.
const TopicAudit = "audit.log"

// AuditEvent is one write/delete action performed by a user. Reads are never
// audited — this middleware only fires for POST/PATCH/PUT/DELETE.
type AuditEvent struct {
	Time       time.Time `json:"time"`        // when it happened
	UserID     string    `json:"user_id"`     // who (empty for api-key/public)
	Role       string    `json:"role"`        // actor role, if known
	AuthMethod string    `json:"auth_method"` // "jwt" | "api_key" | ""
	Action     string    `json:"action"`      // CREATE | UPDATE | DELETE
	Menu       string    `json:"menu"`        // feature/resource, e.g. "agents"
	Method     string    `json:"method"`      // raw HTTP method
	Path       string    `json:"path"`        // request path
	Status     int       `json:"status"`      // response status code
}

// Audit publishes an AuditEvent to Kafka for every successful (2xx) write or
// delete. It derives the action from the HTTP method and the menu from the
// first path segment after /api, so new endpoints are covered automatically
// with no per-route wiring.
//
// ponytail: middleware-level audit — captures user/action/menu/time from the
// request. It cannot see the entity's human name or before/after values; move
// to per-handler publishing if that granularity is ever required.
func Audit(kafka *lib.KafkaProducer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			action := actionFor(r.Method)
			if action == "" {
				next.ServeHTTP(w, r) // read (GET/OPTIONS/HEAD) — not audited
				return
			}

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			// Only record actions that actually succeeded.
			if sw.status < 200 || sw.status >= 300 {
				return
			}

			event := AuditEvent{
				Time:       time.Now().UTC(),
				Action:     action,
				Menu:       menuFor(r.URL.Path),
				Method:     r.Method,
				Path:       r.URL.Path,
				Status:     sw.status,
				AuthMethod: authMethod(r.Context()),
			}
			if claims, ok := ClaimsFromContext(r.Context()); ok {
				event.UserID = claims.UserID.String()
				event.Role = claims.Role
			}

			// Fire-and-forget: a slow broker must never fail the user's write.
			_ = kafka.Publish(context.Background(), TopicAudit, event)
		})
	}
}

func actionFor(method string) string {
	switch method {
	case http.MethodPost:
		return "CREATE"
	case http.MethodPatch, http.MethodPut:
		return "UPDATE"
	case http.MethodDelete:
		return "DELETE"
	default:
		return ""
	}
}

// menuFor returns the resource segment of the path, e.g. "/api/agents/123" ->
// "agents". Falls back to the full path if it can't be parsed.
func menuFor(path string) string {
	p := strings.TrimPrefix(path, "/api/")
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		return path
	}
	return p
}

func authMethod(ctx context.Context) string {
	if m, ok := ctx.Value(AuthMethodCtxKey).(string); ok {
		return m
	}
	return ""
}

// statusWriter records the response status code so the middleware knows whether
// the write succeeded.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true // implicit 200 on first write with no explicit header
	return w.ResponseWriter.Write(b)
}
