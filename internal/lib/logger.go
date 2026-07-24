package lib

import (
	"context"
	"log/slog"
	"os"
)

// logCtxKey is the context key type for values the logger auto-attaches.
type logCtxKey string

const (
	logRequestIDKey logCtxKey = "log_request_id"
	logUserIDKey    logCtxKey = "log_user_id"
)

// WithLogRequestID / WithLogUserID stash trace fields on the context so every
// slog line emitted while handling the request carries them — no need to thread
// fields through every call site.
func WithLogRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, logRequestIDKey, id)
}

func WithLogUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, logUserIDKey, id)
}

// contextHandler wraps a slog.Handler and injects request_id / user_id from the
// context into each record, so `slog.InfoContext(ctx, ...)` traces the user
// uniformly across the whole app.
type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v, ok := ctx.Value(logRequestIDKey).(string); ok && v != "" {
		r.AddAttrs(slog.String("request_id", v))
	}
	if v, ok := ctx.Value(logUserIDKey).(string); ok && v != "" {
		r.AddAttrs(slog.String("user_id", v))
	}
	return h.Handler.Handle(ctx, r)
}

// InitLogger installs a JSON slog logger as the process default. level is one of
// debug/info/warn/error (defaults to info). Call once at startup.
func InitLogger(level string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(contextHandler{base}))
}
