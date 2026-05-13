package app

import (
	"aiac-service/internal/app/middleware"
	"aiac-service/internal/handler"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func AppRouter(conn *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.Cors)

	r.Get("/health", handler.Health)
	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {})
		r.Group(func(r chi.Router) {
			r.Route("/account", func(r chi.Router) {})
			r.Route("/users", func(r chi.Router) {})
			r.Route("/agents", func(r chi.Router) {})
			r.Route("/knowledges", func(r chi.Router) {})
			r.Route("/mcps", func(r chi.Router) {})
			// r.Route("/lookup", func(r chi.Router) {
			// })
		})
	})

	return r
}
