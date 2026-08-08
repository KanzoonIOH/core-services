package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DashboardHandler owns the customizable-dashboard endpoints.
//
// Ownership model: every dashboard has a single owner_id. Only the owner may
// edit/delete it. Admins may publish an owner's dashboard for team use; once
// published, any user can import it. Imports are references (dashboard_imports),
// not copies — an imported dashboard always renders from the owner's live
// config, so owner edits sync to importers instantly and importers are
// read-only by construction.
type DashboardHandler struct {
	Queries db.Querier
}

func NewDashboardHandler(conn *pgxpool.Pool) *DashboardHandler {
	return &DashboardHandler{Queries: db.New(conn)}
}

// defaultConfig is stored when a dashboard is created without an explicit
// config, so the column is never SQL NULL and the widgets array is iterable.
func marshalConfig(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{"widgets": []}`)
	}
	return raw
}

// currentUser pulls the authenticated user's id + role from the JWT claims.
func currentUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, "", false
	}
	return claims.UserID, claims.Role, true
}

func isAdmin(role string) bool {
	return role == string(db.UserRoleADMIN) || role == string(db.UserRoleSUPERADMIN)
}

type dashboardCreateRequest struct {
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Config      json.RawMessage `json:"config"`
}

func (h *DashboardHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}

	var req dashboardCreateRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	dashboard, err := h.Queries.InsertDashboard(r.Context(), db.InsertDashboardParams{
		OwnerID:     userID,
		Name:        req.Name,
		Description: req.Description,
		Config:      marshalConfig(req.Config),
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: create failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboard, nil)
}

// Read returns everything the caller can see: their own dashboards plus any
// published dashboards they've imported.
func (h *DashboardHandler) Read(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}

	dashboards, err := h.Queries.SelectMyDashboards(r.Context(), userID)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: read failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get dashboards")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboards, nil)
}

// ReadPublished returns the team catalog of published dashboards.
func (h *DashboardHandler) ReadPublished(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}

	dashboards, err := h.Queries.SelectPublishedDashboards(r.Context(), userID)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: read published failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get published dashboards")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboards, nil)
}

// ReadById returns a single dashboard if the caller is allowed to see it:
// the owner, an importer, or anyone when it's published.
func (h *DashboardHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	dashboard, err := h.Queries.SelectDashboardById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "dashboard not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get dashboard")
		return
	}

	if !h.canView(r.Context(), dashboard, userID) {
		lib.ResponseJSONError(w, http.StatusForbidden, "you do not have access to this dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboard, nil)
}

// canView is true when the caller owns the dashboard, it's published, or they
// have an import row for it.
func (h *DashboardHandler) canView(ctx context.Context, d db.DashboardsView, userID uuid.UUID) bool {
	if d.OwnerID == userID || d.Visibility == "published" {
		return true
	}
	_, err := h.Queries.SelectDashboardImport(ctx, db.SelectDashboardImportParams{
		DashboardID: d.ID,
		UserID:      userID,
	})
	return err == nil
}

type dashboardUpdateRequest struct {
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Config      json.RawMessage `json:"config"`
}

// Update edits a dashboard. Owner-only: the query is scoped by owner_id, so a
// non-owner gets 0 rows -> 404/forbidden.
func (h *DashboardHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req dashboardUpdateRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	dashboard, err := h.Queries.UpdateDashboard(r.Context(), db.UpdateDashboardParams{
		Name:        req.Name,
		Description: req.Description,
		Config:      marshalConfig(req.Config),
		ID:          id,
		OwnerID:     userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either it doesn't exist or the caller isn't the owner.
			lib.ResponseJSONError(w, http.StatusForbidden, "only the owner can edit this dashboard")
			return
		}
		slog.ErrorContext(r.Context(), "dashboards: update failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboard, nil)
}

// Delete removes a dashboard. Owner-only (query scoped by owner_id).
func (h *DashboardHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteDashboard(r.Context(), db.SoftDeleteDashboardParams{
		ID:      id,
		OwnerID: userID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: delete failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete dashboard")
		return
	}
	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusForbidden, "only the owner can delete this dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

// Publish makes a dashboard available to the team. Admin-only (route is gated
// by RequireRole, and we double-check here defensively).
func (h *DashboardHandler) Publish(w http.ResponseWriter, r *http.Request) {
	userID, role, ok := currentUser(w, r)
	if !ok {
		return
	}
	if !isAdmin(role) {
		lib.ResponseJSONError(w, http.StatusForbidden, "only an admin can publish dashboards")
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	// Ensure it exists before publishing (nicer 404 than a silent no-op).
	if _, err := h.Queries.SelectDashboardById(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "dashboard not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get dashboard")
		return
	}

	publishedBy := userID
	dashboard, err := h.Queries.PublishDashboard(r.Context(), db.PublishDashboardParams{
		PublishedBy: &publishedBy,
		ID:          id,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: publish failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to publish dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboard, nil)
}

// Unpublish reverts a dashboard to private. Allowed for an admin or the owner.
// Existing import rows are left intact but the dashboard drops out of the
// catalog and importers lose access (canView no longer matches).
func (h *DashboardHandler) Unpublish(w http.ResponseWriter, r *http.Request) {
	userID, role, ok := currentUser(w, r)
	if !ok {
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	dashboard, err := h.Queries.SelectDashboardById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "dashboard not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get dashboard")
		return
	}

	if !isAdmin(role) && dashboard.OwnerID != userID {
		lib.ResponseJSONError(w, http.StatusForbidden, "only an admin or the owner can unpublish this dashboard")
		return
	}

	updated, err := h.Queries.UnpublishDashboard(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: unpublish failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to unpublish dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, updated, nil)
}

// Import subscribes the caller to a published dashboard. Idempotent.
func (h *DashboardHandler) Import(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	dashboard, err := h.Queries.SelectDashboardById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "dashboard not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get dashboard")
		return
	}

	if dashboard.Visibility != "published" {
		lib.ResponseJSONError(w, http.StatusForbidden, "this dashboard is not published")
		return
	}
	if dashboard.OwnerID == userID {
		lib.ResponseJSONError(w, http.StatusBadRequest, "you already own this dashboard")
		return
	}

	if _, err := h.Queries.InsertDashboardImport(r.Context(), db.InsertDashboardImportParams{
		DashboardID: id,
		UserID:      userID,
	}); err != nil {
		slog.ErrorContext(r.Context(), "dashboards: import failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to import dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, dashboard, nil)
}

// Unimport removes the caller's subscription to a dashboard.
func (h *DashboardHandler) Unimport(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.DeleteDashboardImport(r.Context(), db.DeleteDashboardImportParams{
		DashboardID: id,
		UserID:      userID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboards: unimport failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to remove dashboard")
		return
	}
	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "you have not imported this dashboard")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
