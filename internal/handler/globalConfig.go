package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GlobalConfigHandler manages the app-wide singleton config that is injected
// into every agent's webhook call as a system prompt.
type GlobalConfigHandler struct {
	Queries db.Querier
}

func NewGlobalConfigHandler(conn *pgxpool.Pool) *GlobalConfigHandler {
	return &GlobalConfigHandler{Queries: db.New(conn)}
}

// Read returns the single global config row (seeded by migration).
func (h *GlobalConfigHandler) Read(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Queries.GetGlobalConfig(r.Context())
	if err != nil {
		log.Printf("[core-service][global-config-read] db select error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get global config")
		return
	}
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, cfg, nil)
}

type updateGlobalConfigRequest struct {
	AgentName           string `json:"agent_name"`
	IndustryDescription string `json:"industry_description"`
	Guardrail           string `json:"guardrail"`
}

func (h *GlobalConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateGlobalConfigRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	cfg, err := h.Queries.UpdateGlobalConfig(r.Context(), db.UpdateGlobalConfigParams{
		AgentName:           strings.TrimSpace(req.AgentName),
		IndustryDescription: strings.TrimSpace(req.IndustryDescription),
		Guardrail:           strings.TrimSpace(req.Guardrail),
	})
	if err != nil {
		log.Printf("[core-service][global-config-update] db update error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update global config")
		return
	}
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, cfg, nil)
}
