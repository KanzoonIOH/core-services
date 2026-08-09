package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PinHandler owns the app-wide pin endpoints. A pin is a per-user shortcut to a
// dashboard, agent, orchestrator, or chat, surfaced in the sidebar "Pinned"
// section. Pins store only a reference; labels/images are resolved live so
// renames propagate automatically.
type PinHandler struct {
	Queries db.Querier
}

func NewPinHandler(conn *pgxpool.Pool) *PinHandler {
	return &PinHandler{Queries: db.New(conn)}
}

// validEntityTypes mirrors the CHECK constraint on the pins table.
var validEntityTypes = map[string]bool{
	"dashboard":    true,
	"agent":        true,
	"orchestrator": true,
	"chat":         true,
}

// routeFor maps an entity type to its frontend route pattern. The sidebar uses
// this to build the link target for each resolved pin.
func routeFor(entityType string, id uuid.UUID) string {
	switch entityType {
	case "dashboard":
		return "/dashboards/" + id.String()
	case "agent":
		return "/agents/garden/" + id.String()
	case "orchestrator":
		return "/agents/orchestrator/" + id.String()
	case "chat":
		return "/chat/" + id.String()
	default:
		return ""
	}
}

type pinRequest struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
}

func (h *PinHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req pinRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.EntityType = strings.TrimSpace(req.EntityType)
	if !validEntityTypes[req.EntityType] {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid entity_type")
		return
	}
	entityID, err := uuid.Parse(strings.TrimSpace(req.EntityID))
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid entity_id")
		return
	}

	pin, err := h.Queries.InsertPin(r.Context(), db.InsertPinParams{
		UserID:     claims.UserID,
		EntityType: req.EntityType,
		EntityID:   entityID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "pins: create failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to pin")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, pin, nil)
}

// Delete unpins by entity reference (entity_type + entity_id in the body), so
// the frontend can unpin from a detail page without knowing the pin's own id.
func (h *PinHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req pinRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.EntityType = strings.TrimSpace(req.EntityType)
	if !validEntityTypes[req.EntityType] {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid entity_type")
		return
	}
	entityID, err := uuid.Parse(strings.TrimSpace(req.EntityID))
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid entity_id")
		return
	}

	rows, err := h.Queries.DeletePin(r.Context(), db.DeletePinParams{
		UserID:     claims.UserID,
		EntityType: req.EntityType,
		EntityID:   entityID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "pins: delete failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to unpin")
		return
	}
	if rows == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "pin not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

// resolvedPin is the sidebar-ready shape: a resolved label + a ready-to-use
// frontend route, so the client renders links without knowing entity routing.
type resolvedPin struct {
	ID         uuid.UUID `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   uuid.UUID `json:"entity_id"`
	Label      string    `json:"label"`
	Image      *string   `json:"image"`
	Route      string    `json:"route"`
	Position   int32     `json:"position"`
}

// Read returns the caller's pins resolved for the sidebar. Pins whose target
// was deleted (unresolved) are silently dropped.
func (h *PinHandler) Read(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.Queries.SelectResolvedPins(r.Context(), claims.UserID)
	if err != nil {
		slog.ErrorContext(r.Context(), "pins: read failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get pins")
		return
	}

	out := make([]resolvedPin, 0, len(rows))
	for _, row := range rows {
		if !row.Resolved {
			continue
		}
		out = append(out, resolvedPin{
			ID:         row.ID,
			EntityType: row.EntityType,
			EntityID:   row.EntityID,
			Label:      row.Label,
			Image:      row.Image,
			Route:      routeFor(row.EntityType, row.EntityID),
			Position:   row.Position,
		})
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, out, nil)
}

// ReadIds returns the bare (entity_type, entity_id) pairs the caller has pinned,
// so the client can render pin-button state cheaply across list pages.
func (h *PinHandler) ReadIds(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.Queries.SelectPinnedEntityIds(r.Context(), claims.UserID)
	if err != nil {
		slog.ErrorContext(r.Context(), "pins: read ids failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get pins")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, rows, nil)
}

type reorderRequest struct {
	// Ordered list of pin ids in the desired sidebar order.
	PinIDs []string `json:"pin_ids"`
}

// Reorder rewrites the position of each pin to match the given order. Pins not
// belonging to the caller are ignored (the update is user-scoped).
func (h *PinHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req reorderRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	for i, raw := range req.PinIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if _, err := h.Queries.UpdatePinPosition(r.Context(), db.UpdatePinPositionParams{
			Position: int32(i),
			ID:       id,
			UserID:   claims.UserID,
		}); err != nil {
			slog.ErrorContext(r.Context(), "pins: reorder failed", "pin_id", id, "error", err)
		}
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
