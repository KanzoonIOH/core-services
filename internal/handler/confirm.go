package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ConfirmHandler struct {
	Queries db.Querier
}

func NewConfirmHandler(conn *pgxpool.Pool) *ConfirmHandler {
	return &ConfirmHandler{
		Queries: db.New(conn),
	}
}

func (h *ConfirmHandler) UpdateEmailConfirm(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	token := lib.ParseParamsString(params, "token")
	if token == nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "Can't parse token")
		return
	}

	upc, err := h.Queries.SelectUpcomingChangeByToken(r.Context(), *token)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "invalid request")
		return
	}

	user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		Email: upc.UpcomingValue,
		ID:    upc.UserID,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "invalid request")
		return
	}

	if err := h.Queries.RevokeUpcomingChangeByID(r.Context(), upc.ID); err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to revoke")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, user, nil)
}
