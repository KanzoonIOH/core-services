package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ExecQHandler struct {
	Queries db.Querier
}

func NewExecQHandler(conn *pgxpool.Pool) *ExecQHandler {
	return &ExecQHandler{Queries: db.New(conn)}
}

type changePasswordRequest struct {
	QueueID  string `json:"queue_id"`
	Password string `json:"password"`
}

func (h *ExecQHandler) Password(w http.ResponseWriter, r *http.Request) {
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, "under construction", nil)
	return

	var req changePasswordRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.QueueID = strings.TrimSpace(req.QueueID)
	req.Password = strings.TrimSpace(req.Password)
	if req.QueueID == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "login_id are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "password are required")
		return
	}

	// TODO: Search on queue.
	// chgq, err := h.Queries.SelectChangeQueueByID(r.Context(), req.QueueID)
	// if err != nil {
	// 	fmt.Printf("%v", err)
	// 	lib.ResponseJSON(w, http.StatusInternalServerError, "link expired")
	// 	return
	// }

	// hashedPassword, err := lib.HashPassword(req.Password)
	// if err != nil {
	// 	lib.ResponseJSON(w, http.StatusBadRequest, err)
	// 	return
	// }

	// user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
	// 	HashedPassword: &hashedPassword,
	// 	ID:             chgq.UserID,
	// })
	// if err != nil {
	// 	fmt.Printf("%v", err)
	// 	lib.ResponseJSON(w, http.StatusInternalServerError, "user not found")
	// 	return
	// }

	// if err != nil {
	// 	fmt.Printf("%v", err)
	// 	lib.ResponseJSON(w, http.StatusInternalServerError, "failed to update user")
	// 	return
	// }

	// lib.ResponseJSON(w, http.StatusOK, user)
}

type changeEmailRequest struct {
	QueueID string `json:"queue_id"`
}

func (h *ExecQHandler) Email(w http.ResponseWriter, r *http.Request) {
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, "under construction", nil)
	return

	var req changeEmailRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.QueueID = strings.TrimSpace(req.QueueID)
	if req.QueueID == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "login_id are required")
		return
	}
}
