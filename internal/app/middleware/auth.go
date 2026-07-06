package middleware

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ctxKey string

const (
	ClaimsCtxKey     ctxKey = "jwtClaims"
	AuthMethodCtxKey ctxKey = "authMethod"
	AuthMethodJWT           = "jwt"
	AuthMethodApiKey        = "api_key"
)

func Auth(signer *lib.JWTSigner) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(w, r)
			if !ok {
				return
			}

			claims, err := signer.Verify(token)
			if err != nil {
				lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsCtxKey, claims)
			ctx = context.WithValue(ctx, AuthMethodCtxKey, AuthMethodJWT)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AuthOrApiKey(signer *lib.JWTSigner, queries db.Querier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(w, r)
			if !ok {
				return
			}

			claims, err := signer.Verify(token)
			if err == nil {
				ctx := context.WithValue(r.Context(), ClaimsCtxKey, claims)
				ctx = context.WithValue(ctx, AuthMethodCtxKey, AuthMethodJWT)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			_, err = queries.SelectApiKeyByToken(r.Context(), token)
			if err == nil {
				ctx := context.WithValue(r.Context(), AuthMethodCtxKey, AuthMethodApiKey)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if errors.Is(err, pgx.ErrNoRows) {
				lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid bearer token")
				return
			}

			lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to verify api key")
		})
	}
}

// InternalKey guards internal service-to-service routes with a static key from
// env, passed in the X-Internal-Key header. No login, no JWT.
// ponytail: constant-time compare avoids timing leaks; static key, rotate via env.
func InternalKey(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Internal-Key")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid internal key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole gates a route to callers whose JWT role is in the allowed set.
// Must run after Auth (needs claims in context). API-key auth has no role and
// is always rejected.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				lib.ResponseJSONError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			if _, ok := allowed[claims.Role]; !ok {
				lib.ResponseJSONError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "missing authorization header")
		return "", false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid authorization header")
		return "", false
	}

	return strings.TrimSpace(parts[1]), true
}

func ClaimsFromContext(ctx context.Context) (*lib.JWTClaims, bool) {
	claims, ok := ctx.Value(ClaimsCtxKey).(*lib.JWTClaims)
	return claims, ok
}

func IsApiKeyAuth(ctx context.Context) bool {
	authMethod, ok := ctx.Value(AuthMethodCtxKey).(string)
	return ok && authMethod == AuthMethodApiKey
}
