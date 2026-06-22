package handler

import (
	"aic3-service/internal/lib"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type RouteInfo struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
	Auth    bool   `json:"auth_required"`
}

func AllFunctions(r chi.Router) http.HandlerFunc {
	publicPrefixes := []string{
		"/health",
		"/all-functions",
		"/api/auth/",
		"/api/exec/",
	}

	isPublic := func(pattern string) bool {
		for _, prefix := range publicPrefixes {
			if len(pattern) >= len(prefix) && pattern[:len(prefix)] == prefix {
				return true
			}
			if pattern == prefix[:len(prefix)-1] {
				return true
			}
		}
		// /health exact
		if pattern == "/health" {
			return true
		}
		return false
	}

	return func(w http.ResponseWriter, req *http.Request) {
		var routes []RouteInfo

		chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			routes = append(routes, RouteInfo{
				Method:  method,
				Pattern: route,
				Auth:    !isPublic(route),
			})
			return nil
		})

		lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
			"total":  len(routes),
			"routes": routes,
		}, nil)
	}
}
