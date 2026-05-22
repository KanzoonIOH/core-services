package app

import (
	"aiac-service/internal/app/middleware"
	"aiac-service/internal/handler"
	"aiac-service/internal/lib"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

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

	signer := newJWTSigner()
	mailer := newMailer()

	agentHandler := handler.NewAgentHandler(conn)
	mcpHandler := handler.NewMcpHandler(conn)
	knowledgeHandler := handler.NewKnowledgeHandler(conn)
	agentKnowledgeHandler := handler.NewAgentKnowledgeHandler(conn)
	authHandler := handler.NewAuthHandler(conn, signer, mailer)
	meHandler := handler.NewMeHandler(conn, mailer)
	confirmHandler := handler.NewConfirmHandler(conn)
	apiKeyHandler := handler.NewApiKeyHandler(conn)
	webhookHandler := handler.NewWebhookHandler(conn)

	r.Get("/health", handler.Health)
	r.Get("/all-functions", handler.AllFunctions(r))
	r.Get("/email-tester", handler.MailerDummy(r, mailer))
	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/password/forgot", authHandler.ForgotPassword)
			r.Post("/password/reset", authHandler.ResetPassword)
		})
		r.Get("/confirm", confirmHandler.UpdateEmailConfirm)
		r.Route("/", func(r chi.Router) {
			r.Use(middleware.AuthOrApiKey(signer, webhookHandler.Queries))
			// No timeout here — webhook forwards to upstream and may take a long time
			r.Post("/chat/{id}", webhookHandler.ForwardChatWebhook)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(signer))
			r.Route("/me", func(r chi.Router) {
				r.Get("/", meHandler.Read)
				r.Patch("/", meHandler.UpdateDetails)
				r.Patch("/password", meHandler.UpdatePassword)
				r.Patch("/email", meHandler.UpdateEmailRequest)
			})
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
			r.Route("/api-keys", func(r chi.Router) {
				r.Post("/", apiKeyHandler.Create)
				r.Get("/", apiKeyHandler.Read)
				r.Delete("/{id}", apiKeyHandler.Delete)
			})
		})
	})

	return r
}

func newJWTSigner() *lib.JWTSigner {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	issuer := os.Getenv("SECRET_ISSUER")
	if issuer == "" {
		log.Fatal("SECRET_ISSUER is required")
	}
	ttlDaysStr := os.Getenv("ACCESS_TOKEN_TTL_DAYS")
	if ttlDaysStr == "" {
		log.Fatal("ACCESS_TOKEN_TTL_DAYS is required")
	}
	ttlDays, err := strconv.Atoi(ttlDaysStr)
	if err != nil {
		log.Fatalf("invalid ACCESS_TOKEN_TTL_DAYS: %v", err)
	}

	return lib.NewJWTSigner(secret, issuer, time.Duration(ttlDays)*24*time.Hour)
}

func newMailer() *lib.Mailer {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Fatal("RESEND_API_KEY is required")
	}
	from := os.Getenv("RESEND_FROM")
	if from == "" {
		log.Fatal("RESEND_FROM is required")
	}
	appUrl := os.Getenv("APP_URL")
	if appUrl == "" {
		log.Fatal("APP_URL is required")
	}
	return lib.NewMailer(apiKey, from, appUrl)
}
