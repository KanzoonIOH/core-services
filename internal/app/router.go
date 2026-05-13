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

	agentHandler := handler.NewAgentHandler(conn)
	mcpHandler := handler.NewMcpHandler(conn)
	knowledgeHandler := handler.NewKnowledgeHandler(conn)
	agentKnowledgeHandler := handler.NewAgentKnowledgeHandler(conn)
	authHandler := handler.NewAuthHandler(conn)

	r.Get("/health", handler.Health)
	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
		})
		r.Group(func(r chi.Router) {
			r.Route("/account", func(r chi.Router) {})
			r.Route("/users", func(r chi.Router) {})
			r.Route("/agents", func(r chi.Router) {
				r.Post("/", agentHandler.Create)
				r.Get("/", agentHandler.Read)
				r.Get("/{id}", agentHandler.ReadById)
				r.Patch("/{id}", agentHandler.Update)
				r.Delete("/{id}", agentHandler.Delete)
			})
			r.Route("/knowledges", func(r chi.Router) {
				r.Post("/", knowledgeHandler.Create)
				r.Get("/", knowledgeHandler.Read)
				r.Get("/{id}", knowledgeHandler.ReadById)
				r.Patch("/{id}", knowledgeHandler.Update)
				r.Delete("/{id}", knowledgeHandler.Delete)
			})
			r.Route("/mcps", func(r chi.Router) {
				r.Post("/", mcpHandler.Create)
				r.Get("/", mcpHandler.Read)
				r.Get("/{id}", mcpHandler.ReadById)
				r.Patch("/{id}", mcpHandler.Update)
				r.Delete("/{id}", mcpHandler.Delete)
			})
			r.Route("/agent-knowledges", func(r chi.Router) {
				r.Post("/agent/{id}", agentKnowledgeHandler.CreateByAgentId)
				r.Get("/agent/{id}", agentKnowledgeHandler.ReadByAgentId)
				r.Patch("/{id}", agentKnowledgeHandler.Update)
				r.Delete("/{id}", agentKnowledgeHandler.Delete)
			})
		})
	})

	return r
}
