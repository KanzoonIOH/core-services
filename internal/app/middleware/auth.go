package middleware

import (
	"aiac-service/internal/lib"
	"context"
	"net/http"
	"strings"
)

type ctxKey string

const ClaimsCtxKey ctxKey = "jwtClaims"

func Auth(signer *lib.JWTSigner) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				lib.ResponseJSON(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				lib.ResponseJSON(w, http.StatusUnauthorized, "invalid authorization header")
				return
			}

			claims, err := signer.Verify(strings.TrimSpace(parts[1]))
			if err != nil {
				lib.ResponseJSON(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFromContext(ctx context.Context) (*lib.JWTClaims, bool) {
	claims, ok := ctx.Value(ClaimsCtxKey).(*lib.JWTClaims)
	return claims, ok
}
