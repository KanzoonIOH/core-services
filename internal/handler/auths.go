package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	Queries db.Querier
	Signer  *lib.JWTSigner
	Mailer  *lib.Mailer
}

func NewAuthHandler(conn *pgxpool.Pool, signer *lib.JWTSigner, mailer *lib.Mailer) *AuthHandler {
	return &AuthHandler{
		Queries: db.New(conn),
		Signer:  signer,
		Mailer:  mailer,
	}
}

type registerAuthRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerAuthRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "username are required")
		return
	}
	if req.Email == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "email are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "password are required")
		return
	}

	hashedPassword, err := lib.HashPassword(req.Password)
	if err != nil {
		lib.ResponseJSON(w, http.StatusBadRequest, err)
		return
	}

	user, err := h.Queries.InsertUserRegister(r.Context(), db.InsertUserRegisterParams{
		Username:       req.Username,
		Email:          req.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to register user")
		return
	}

	token, err := h.Signer.Issue(user.ID, string(user.Role))
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, map[string]any{
		"user":  user,
		"token": token,
	})
}

type loginAuthRequest struct {
	LoginID  string `json:"login_id"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginAuthRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.LoginID = strings.TrimSpace(req.LoginID)
	req.Password = strings.TrimSpace(req.Password)
	if req.LoginID == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "login_id are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "password are required")
		return
	}

	user, err := h.Queries.SelectUserByLoginId(r.Context(), req.LoginID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "user not found")
		return
	}

	if !lib.ComparePassword(user.HashedPassword, req.Password) {
		lib.ResponseJSON(w, http.StatusInternalServerError, "Password is incorrect")
		return

	}

	token, err := h.Signer.Issue(user.ID, string(user.Role))
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, map[string]any{
		"user":  user,
		"token": token,
	})
}

type forgotPasswordRequest struct {
	LoginID string `json:"login_id"`
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.LoginID = strings.TrimSpace(req.LoginID)
	if req.LoginID == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "login_id are required")
		return
	}

	const safeResponse = "If an account exists for that login, a password reset link has been sent"

	user, err := h.Queries.SelectUserByLoginId(r.Context(), req.LoginID)
	if err != nil {
		lib.ResponseJSON(w, http.StatusOK, safeResponse)
		return
	}

	upc, err := h.Queries.InsertUpcomingChange(r.Context(), db.InsertUpcomingChangeParams{
		Type:   db.UpcomingChangesTypeForgotPassword,
		UserID: user.ID,
	})
	if err != nil {
		fmt.Printf("forgot-password: insert upcoming change: %v\n", err)
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	url := h.Mailer.IssueURL(upc.Token)
	body := fmt.Sprintf(
		"Hi %s,\n\nClick the link below to reset your password:\n\n%s\n\nThis link expires in 1 hour.\n\nIf it is not you, please ignore this message.",
		user.Name, url,
	)

	if err := h.Mailer.Send(r.Context(), user.Email, "Confirm Your Forgot Password Request", body); err != nil {
		fmt.Printf("forgot-password: send email: %v\n", err)
	}

	lib.ResponseJSON(w, http.StatusOK, safeResponse)
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"new_password"`
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	req.Password = strings.TrimSpace(req.Password)
	if req.Token == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "token are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSON(w, http.StatusBadRequest, "new_password are required")
		return
	}

	upc, err := h.Queries.SelectUpcomingChangeByToken(r.Context(), req.Token)
	if err != nil {
		lib.ResponseJSON(w, http.StatusInternalServerError, "invalid request")
		return
	}

	hashedPassword, err := lib.HashPassword(req.Password)
	if err != nil {
		lib.ResponseJSON(w, http.StatusBadRequest, err)
		return
	}

	user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		HashedPassword: &hashedPassword,
		ID:             upc.UserID,
	})
	if err != nil {
		lib.ResponseJSON(w, http.StatusInternalServerError, "invalid request")
		return
	}

	if err := h.Queries.RevokeUpcomingChangeByID(r.Context(), upc.ID); err != nil {
		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to revoke")
		return
	}

	lib.ResponseJSON(w, http.StatusOK, user)
}
