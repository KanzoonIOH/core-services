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
}

func NewAuthHandler(conn *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{Queries: db.New(conn)}
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

	lib.ResponseJSON(w, http.StatusOK, user)
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

	lib.ResponseJSON(w, http.StatusOK, user)
}
