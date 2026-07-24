package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// canEditMember encodes who may modify (role change / accept / remove) a target
// member. Rules: nobody edits themselves; nobody edits a SUPERADMIN; an ADMIN
// cannot edit another ADMIN. Only ADMIN/SUPERADMIN can edit at all (route is
// already role-gated, so actorRole is one of those here).
func canEditMember(actorID uuid.UUID, actorRole string, targetID uuid.UUID, targetRole db.UserRole) bool {
	if actorID == targetID {
		return false
	}
	if targetRole == db.UserRoleSUPERADMIN {
		return false
	}
	if actorRole == string(db.UserRoleADMIN) && targetRole == db.UserRoleADMIN {
		return false
	}
	return true
}

// authorizeMemberEdit loads the target member's role and applies canEditMember.
// Writes a 403/404 response and returns false when the edit is not allowed.
func (h *MemberHandler) authorizeMemberEdit(w http.ResponseWriter, r *http.Request, targetID uuid.UUID) bool {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}

	target, err := h.Queries.SelectUserById(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "member not found")
			return false
		}
		slog.ErrorContext(r.Context(), "members: load member failed", "member_id", targetID, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to load member")
		return false
	}

	if !canEditMember(claims.UserID, claims.Role, targetID, target.Role) {
		lib.ResponseJSONError(w, http.StatusForbidden, "you are not allowed to edit this member")
		return false
	}
	return true
}

type MemberHandler struct {
	Queries db.Querier
}

func NewMemberHandler(conn *pgxpool.Pool) *MemberHandler {
	return &MemberHandler{Queries: db.New(conn)}
}

func (h *MemberHandler) Read(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	role := lib.ParseParamsString(params, "role")

	members, err := h.Queries.SelectMembers(r.Context(), db.SelectMembersParams{
		Role:   role,
		Sort:   pagination.Sort,
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "members: read failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get members")
		return
	}

	totalRow, err := h.Queries.CountMembers(r.Context(), role)
	if err != nil {
		slog.ErrorContext(r.Context(), "members: count failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get members")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, members, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(members), int(totalRow)))
}

func (h *MemberHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}
	if !h.authorizeMemberEdit(w, r, id) {
		return
	}

	member, err := h.Queries.AcceptMember(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "member not found or already accepted")
			return
		}
		slog.ErrorContext(r.Context(), "members: accept failed", "member_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to accept member")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, member, nil)
}

type updateMemberStatusRequest struct {
	Role db.UserRole `json:"role"`
}

func (h *MemberHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	if !h.authorizeMemberEdit(w, r, id) {
		return
	}

	var req updateMemberStatusRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	switch req.Role {
	case db.UserRoleADMIN, db.UserRoleVIEWER, db.UserRoleTECHNICAL:
		// valid
	default:
		lib.ResponseJSONError(w, http.StatusBadRequest, "role must be one of: ADMIN, VIEWER, TECHNICAL")
		return
	}

	member, err := h.Queries.UpdateMemberStatus(r.Context(), db.UpdateMemberStatusParams{
		Role: req.Role,
		ID:   id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "member not found or cannot change status of a new member")
			return
		}
		slog.ErrorContext(r.Context(), "members: update status failed", "member_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update member status")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, member, nil)
}

func (h *MemberHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}
	if !h.authorizeMemberEdit(w, r, id) {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteMember(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "members: delete failed", "member_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete member")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "member not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
