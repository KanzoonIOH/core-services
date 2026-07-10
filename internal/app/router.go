package app

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/handler"
	"aic3-service/internal/lib"
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func AppRouter(conn *pgxpool.Pool, kafka *lib.KafkaProducer, ch *lib.ClickHouseClient) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.Cors)

	signer := newJWTSigner()
	mailer := newMailer()
	objectStorage := newObjectStorage()

	agentHandler := handler.NewAgentHandler(conn)
	orchestratorHandler := handler.NewOrchestratorHandler(conn)
	tagHandler := handler.NewTagHandler(conn)
	mcpHandler := handler.NewMcpHandler(conn)
	knowledgeHandler := handler.NewKnowledgeHandler(conn, objectStorage)
	agentKnowledgeHandler := handler.NewAgentKnowledgeHandler(conn)
	connectHandler := handler.NewConnectHandler(conn)
	callbackHandler := handler.NewCallbackHandler(conn)
	authHandler := handler.NewAuthHandler(conn, signer, mailer, inviteTTL())
	meHandler := handler.NewMeHandler(conn, mailer, objectStorage)
	confirmHandler := handler.NewConfirmHandler(conn)
	apiKeyHandler := handler.NewApiKeyHandler(conn)
	webhookHandler := handler.NewWebhookHandler(conn, kafka)
	conversationHandler := handler.NewConversationHandler(conn, kafka)
	conversationsHandler := handler.NewConversationsHandler(conn)
	memberHandler := handler.NewMemberHandler(conn)
	logHandler := handler.NewLogHandler(ch)
	dropdownHandler := handler.NewDropdownHandler(conn)
	globalConfigHandler := handler.NewGlobalConfigHandler(conn)
	uploadHandler := handler.NewUploadHandler(conn, objectStorage)

	r.Get("/health", handler.Health)
	r.Get("/all-functions", handler.AllFunctions(r))
	r.Get("/email-tester", handler.MailerDummy(r, mailer))
	r.Route("/api", func(r chi.Router) {
		// r.Post("/registersuperadmin", authHandler.RegisterSuperAdmin)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/password/forgot", authHandler.ForgotPassword)
			r.Post("/password/reset", authHandler.ResetPassword)
			r.Post("/accept-invite", authHandler.AcceptInvite)
		})
		r.Get("/confirm", confirmHandler.UpdateEmailConfirm)
		// Internal service-to-service routes: no login, guarded by a static
		// key (INTERNAL_API_KEY env) sent in the X-Internal-Key header.
		r.Route("/internal", func(r chi.Router) {
			r.Use(middleware.InternalKey(mustEnv("INTERNAL_API_KEY")))
			r.Get("/agent/{id}", agentHandler.ReadById)
			r.Get("/orchestrator/{id}", orchestratorHandler.ReadById)
			r.Get("/mcp/{id}", mcpHandler.ReadByAgentId)
			// Called by the n8n conversion workflow using the internal key.
			r.Patch("/callbacks/agent-knowledge-status", callbackHandler.UpdateAgentKnowledgeStatus)
		})
		// CORS preflight for the widget-embeddable chat endpoint. No auth: a
		// preflight never carries credentials.
		r.Options("/chat/{id}", webhookHandler.PreflightChatWebhook)
		r.Route("/", func(r chi.Router) {
			r.Use(middleware.AuthOrApiKey(signer, webhookHandler.Queries))
			// No timeout here — webhook forwards to upstream and may take a long time
			r.Post("/chat/{id}", webhookHandler.ForwardChatWebhook)
			r.Post("/chat/{id}/stream", webhookHandler.ForwardChatWebhookStream)
			r.Post("/chat/{id}/conversation/end", conversationHandler.EndConversation)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(signer))
			r.Use(middleware.Audit(kafka))
			r.Route("/me", func(r chi.Router) {
				r.Get("/", meHandler.Read)
				r.Patch("/", meHandler.UpdateDetails)
				r.Patch("/password", meHandler.UpdatePassword)
				r.Patch("/email", meHandler.UpdateEmailRequest)
				r.Patch("/avatar", meHandler.UpdateAvatar)
			})
			r.Route("/members", func(r chi.Router) {
				r.Get("/", memberHandler.Read)
				// Role management is admin-only.
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireRole(
						string(db.UserRoleADMIN),
						string(db.UserRoleSUPERADMIN),
					))
					r.Post("/invite", authHandler.Invite)
					r.Patch("/{id}/accept", memberHandler.Accept)
					r.Patch("/{id}/status", memberHandler.UpdateStatus)
					r.Delete("/{id}", memberHandler.Delete)
				})
			})
			r.Route("/agents", func(r chi.Router) {
				r.Post("/", agentHandler.Create)
				r.Get("/", agentHandler.Read)
				r.Get("/{id}", agentHandler.ReadById)
				r.Patch("/{id}", agentHandler.Update)
				r.Delete("/{id}", agentHandler.Delete)
				r.Patch("/{id}/persona", agentHandler.UpdatePersona)
				r.Get("/{id}/mcps", mcpHandler.ReadByAgentId)
				r.Get("/{id}/mcps/all", mcpHandler.ReadAllByAgentId)
				r.Get("/{id}/knowledges", knowledgeHandler.ReadByAgentId)
				r.Get("/{id}/knowledges/all", knowledgeHandler.ReadAllByAgentId)
			})
			r.Route("/orchestrators", func(r chi.Router) {
				r.Post("/", orchestratorHandler.Create)
				r.Get("/", orchestratorHandler.Read)
				r.Get("/{id}", orchestratorHandler.ReadById)
				r.Patch("/{id}", orchestratorHandler.Update)
				r.Delete("/{id}", orchestratorHandler.Delete)
			})
			r.Post("/uploads/image", uploadHandler.Image)
			r.Route("/tags", func(r chi.Router) {
				r.Get("/", tagHandler.Read)
				r.Patch("/{id}", tagHandler.Update)
				r.Delete("/{id}", tagHandler.Delete)
			})
			// Singleton — no {id}.
			r.Route("/global-config", func(r chi.Router) {
				r.Get("/", globalConfigHandler.Read)
				r.Patch("/", globalConfigHandler.Update)
			})
			r.Route("/knowledges", func(r chi.Router) {
				r.Post("/", knowledgeHandler.Create)
				r.Get("/", knowledgeHandler.Read)
				r.Get("/{id}", knowledgeHandler.ReadById)
				r.Patch("/{id}", knowledgeHandler.Update)
				r.Delete("/{id}", knowledgeHandler.Delete)
				r.Get("/{id}/agents", agentHandler.ReadByKnowledgeId)
			})
			r.Route("/mcps", func(r chi.Router) {
				r.Post("/", mcpHandler.Create)
				r.Get("/", mcpHandler.Read)
				r.Get("/{id}", mcpHandler.ReadById)
				r.Patch("/{id}", mcpHandler.Update)
				r.Delete("/{id}", mcpHandler.Delete)
				r.Get("/{id}/tools", mcpHandler.ReadTools)
				r.Post("/{id}/refresh-tools", mcpHandler.RefreshTools)
				r.Get("/{id}/agents", agentHandler.ReadByMcpId)
			})
			r.Route("/agent-knowledges", func(r chi.Router) {
				r.Post("/agent/{id}", agentKnowledgeHandler.CreateByAgentId)
				r.Get("/agent/{id}", agentKnowledgeHandler.ReadByAgentId)
				r.Patch("/{id}", agentKnowledgeHandler.Update)
				r.Delete("/{id}", agentKnowledgeHandler.Delete)
			})
			r.Route("/connect", func(r chi.Router) {
				r.Post("/agent-mcp", connectHandler.ConnectAgentMcp)
				r.Delete("/agent-mcp", connectHandler.DisconnectAgentMcp)
				r.Post("/agent-knowledge", connectHandler.ConnectAgentKnowledge)
				r.Delete("/agent-knowledge", connectHandler.DisconnectAgentKnowledge)
			})
			r.Route("/dropdown", func(r chi.Router) {
				r.Get("/agents", dropdownHandler.ReadAgents)
			})
			r.Route("/conversations", func(r chi.Router) {
				r.Get("/", conversationsHandler.List)
				r.Get("/{id}", conversationsHandler.Read)
				r.Delete("/{id}", conversationsHandler.Delete)
			})
			r.Route("/api-keys", func(r chi.Router) {
				r.Post("/", apiKeyHandler.Create)
				r.Get("/", apiKeyHandler.Read)
				r.Patch("/{id}", apiKeyHandler.Update)
				r.Delete("/{id}", apiKeyHandler.Delete)
			})
			r.Route("/logs", func(r chi.Router) {
				// audit trail (write/delete actions)
				r.Get("/audit", logHandler.ReadAuditLogs)

				// webhook message logs
				r.Get("/messages", logHandler.ReadMessages)
				r.Get("/messages/{id}", logHandler.ReadMessagesByAgentId)
				r.Get("/summary", logHandler.Summary)
				r.Get("/summary-conv", logHandler.ConversationsSummary)
				r.Get("/timeseries", logHandler.Timeseries)
				r.Get("/agents/performance", logHandler.AgentPerformance)
				r.Get("/traffic-heatmap", logHandler.TrafficHeatmap)

				// conversation analytics
				r.Get("/conversations/summary", logHandler.ConversationsSummary)
				r.Get("/conversations/timeseries", logHandler.ConversationsTimeseries)
				r.Get("/conversations/intents", logHandler.IntentStats)
				r.Get("/conversations/topics", logHandler.TopicStats)
				r.Get("/conversations/sentiment", logHandler.SentimentStats)
			})
		})
	})

	return r
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
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

func inviteTTL() time.Duration {
	daysStr := os.Getenv("INVITE_TTL_DAYS")
	if daysStr == "" {
		log.Fatal("INVITE_TTL_DAYS is required")
	}
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		log.Fatalf("invalid INVITE_TTL_DAYS: %v", err)
	}
	return time.Duration(days) * 24 * time.Hour
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

func newObjectStorage() *lib.ObjectStorage {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		log.Fatal("S3_ENDPOINT is required")
	}
	accessKey := os.Getenv("S3_ACCESS_KEY")
	if accessKey == "" {
		log.Fatal("S3_ACCESS_KEY is required")
	}
	secretKey := os.Getenv("S3_SECRET_KEY")
	if secretKey == "" {
		log.Fatal("S3_SECRET_KEY is required")
	}
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET is required")
	}
	region := os.Getenv("S3_REGION")
	if region == "" {
		log.Fatal("S3_REGION is required")
	}

	storage, err := lib.NewObjectStorage(context.Background(), endpoint, os.Getenv("S3_PUBLIC_ENDPOINT"), region, accessKey, secretKey, bucket)
	if err != nil {
		log.Fatalf("object storage: %v", err)
	}

	return storage
}
