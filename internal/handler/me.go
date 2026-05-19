package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/app/middleware"
	"aiac-service/internal/lib"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MeHandler struct {
	Queries db.Querier
	Mailer  *lib.Mailer
}

func NewMeHandler(conn *pgxpool.Pool, mailer *lib.Mailer) *MeHandler {
	return &MeHandler{Queries: db.New(conn), Mailer: mailer}
}

func (h *MeHandler) Read(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	user, err := h.Queries.SelectUserById(r.Context(), userID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, user)
}

type updatePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *MeHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	user, err := h.Queries.SelectUserById(r.Context(), userID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	var req updatePasswordRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.OldPassword = strings.TrimSpace(req.OldPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if req.OldPassword == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "old password are required")
		return
	}
	if req.NewPassword == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "new password are required")
		return
	}
	if req.OldPassword == req.NewPassword {
		lib.ResponseJSON(w, http.StatusBadRequest, "new password has to be different")
		return
	}

	if !lib.ComparePassword(user.HashedPassword, req.OldPassword) {
		lib.ResponseJSON(w, http.StatusInternalServerError, "Password is incorrect")
		return
	}

	hashedPassword, err := lib.HashPassword(req.NewPassword)
	if err != nil {
		lib.ResponseJSON(w, http.StatusBadRequest, err)
		return
	}

	newUser, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		HashedPassword: &hashedPassword,
		ID:             userID,
	})

	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, newUser)
}

type updateDetailsRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
}

func (h *MeHandler) UpdateDetails(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	var req updateDetailsRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)
	if req.Name == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.Username == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "username are required")
		return
	}

	user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		Name:     &req.Name,
		Username: &req.Username,
		ID:       userID,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, user)
}

type updateEmailRequest struct {
	OldEmail string `json:"old_email"`
	NewEmail string `json:"new_email"`
}

func (h *MeHandler) UpdateEmail(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	var req updateEmailRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.OldEmail = strings.TrimSpace(req.OldEmail)
	req.NewEmail = strings.TrimSpace(req.NewEmail)
	if req.OldEmail == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "old email are required")
		return
	}
	if req.NewEmail == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "new email are required")
		return
	}
	if req.OldEmail == req.NewEmail {
		lib.ResponseJSON(w, http.StatusBadRequest, "new email has to be different")
		return
	}

	user, err := h.Queries.SelectUserById(r.Context(), userID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	if user.Email != req.OldEmail {
		lib.ResponseJSON(w, http.StatusBadRequest, "old email does not match")
		return
	}

	// confirmLink := fmt.Sprintf("%s/confirm-email?queue_id=%s", os.Getenv("APP_URL"), queue.ID.String())
	// body := fmt.Sprintf("Hi %s,\n\nClick the link below to confirm your email change:\n\n%s\n\nThis link expires in 1 hour.", user.Name, confirmLink)
	// if err := h.Mailer.Send(r.Context(), user.Email, "Confirm Your Email Change", body); err != nil {
	// 	fmt.Printf("%v", err)
	// 	lib.ResponseJSON(w, http.StatusInternalServerError, "failed to send confirmation email")
	// 	return
	// }

	// lib.ResponseJSON(w, http.StatusOK, "A confirmation link has been sent to your current email address")
}
