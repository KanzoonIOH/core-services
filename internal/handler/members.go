package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get members")
		return
	}

	totalRow, err := h.Queries.CountMembers(r.Context(), role)
	if err != nil {
		fmt.Printf("%v\n", err)
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

	member, err := h.Queries.AcceptMember(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "member not found or already accepted")
			return
		}
		fmt.Printf("%v\n", err)
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
		fmt.Printf("%v\n", err)
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

	rowsAffected, err := h.Queries.SoftDeleteMember(r.Context(), id)
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete member")
		return
	}

	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "member not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
