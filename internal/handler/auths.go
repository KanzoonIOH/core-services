package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	Queries   db.Querier
	Signer    *lib.JWTSigner
	Mailer    *lib.Mailer
	InviteTTL time.Duration
}

func NewAuthHandler(conn *pgxpool.Pool, signer *lib.JWTSigner, mailer *lib.Mailer, inviteTTL time.Duration) *AuthHandler {
	return &AuthHandler{
		Queries:   db.New(conn),
		Signer:    signer,
		Mailer:    mailer,
		InviteTTL: inviteTTL,
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
		lib.ResponseJSONError(w, http.StatusBadRequest, "username are required")
		return
	}
	if req.Email == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "email are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "password are required")
		return
	}

	hashedPassword, err := lib.HashPassword(req.Password)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.Queries.InsertUserRegister(r.Context(), db.InsertUserRegisterParams{
		Username:       req.Username,
		Email:          req.Email,
		HashedPassword: &hashedPassword,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to register user")
		return
	}

	token, err := h.Signer.Issue(user.ID, string(user.Role))
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":  user,
		"token": token,
	}, nil)
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
		lib.ResponseJSONError(w, http.StatusBadRequest, "login_id are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "password are required")
		return
	}

	user, err := h.Queries.SelectUserByLoginIdWithPassword(r.Context(), req.LoginID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "user not found")
		return
	}

	if user.HashedPassword == nil || !lib.ComparePassword(*user.HashedPassword, req.Password) {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "Password is incorrect")
		return
	}

	token, err := h.Signer.Issue(user.ID, string(user.Role))
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":  lib.ToUserResponse(user),
		"token": token,
	}, nil)
}

const resetPasswordPath = "/reset-password"

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
		lib.ResponseJSONError(w, http.StatusBadRequest, "login_id are required")
		return
	}

	const safeResponse = "If an account exists for that login, a password reset link has been sent"

	user, err := h.Queries.SelectUserByLoginIdWithPassword(r.Context(), req.LoginID)
	if err != nil {
		lib.ResponseJSONTemplate(w, http.StatusOK, nil, safeResponse, nil)
		return
	}

	changeToken, err := lib.GenerateSecureToken(32)
	if err != nil {
		fmt.Printf("forgot-password: generate token: %v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	upc, err := h.Queries.InsertUpcomingChange(r.Context(), db.InsertUpcomingChangeParams{
		Token:  changeToken,
		Type:   db.UpcomingChangesTypeFORGOTPASSWORD,
		UserID: user.ID,
	})
	if err != nil {
		fmt.Printf("forgot-password: insert upcoming change: %v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	url := h.Mailer.IssuePathURL(resetPasswordPath, upc.Token)
	body := fmt.Sprintf(
		"Hi %s,\n\nClick the link below to reset your password:\n\n%s\n\nThis link expires in 1 hour.\n\nIf it is not you, please ignore this message.",
		user.Name, url,
	)

	if err := h.Mailer.Send(r.Context(), user.Email, "Confirm Your Forgot Password Request", body); err != nil {
		fmt.Printf("forgot-password: send email: %v\n", err)
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, safeResponse, nil)
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
		lib.ResponseJSONError(w, http.StatusBadRequest, "token are required")
		return
	}
	if req.Password == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "new_password are required")
		return
	}

	upc, err := h.Queries.SelectUpcomingChangeByToken(r.Context(), req.Token)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "invalid request")
		return
	}

	hashedPassword, err := lib.HashPassword(req.Password)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		HashedPassword: &hashedPassword,
		ID:             upc.UserID,
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

// ---------------------------------------------------------------------------
// Invite flow: an admin invites a user by email; the user receives a link and
// sets their own password, which activates them as VIEWER.
// ---------------------------------------------------------------------------

const invitePath = "/invite"

type inviteRequest struct {
	Email string `json:"email"`
}

// Invite creates a passwordless PENDING user and emails them an accept link.
// Requires ADMIN/SUPERADMIN (guarded at the route).
func (h *AuthHandler) Invite(w http.ResponseWriter, r *http.Request) {
	var req inviteRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		lib.ResponseJSONError(w, http.StatusBadRequest, "a valid email is required")
		return
	}

	user, err := h.Queries.InsertUserInvite(r.Context(), req.Email)
	if err != nil {
		// Unique email index -> already invited/registered.
		lib.ResponseJSONError(w, http.StatusConflict, "a user with this email already exists")
		return
	}

	inviteToken, err := lib.GenerateSecureToken(32)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to generate invite token")
		return
	}

	upc, err := h.Queries.InsertUpcomingChange(r.Context(), db.InsertUpcomingChangeParams{
		Token:     inviteToken,
		Type:      db.UpcomingChangesTypeINVITE,
		UserID:    user.ID,
		ExpiredAt: time.Now().Add(h.InviteTTL),
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}

	url := h.Mailer.IssuePathURL(invitePath, upc.Token)
	body := fmt.Sprintf(
		"You have been invited to the AI Customer Care console.\n\nSet your password to activate your account:\n%s\n\nThis link expires in %d days.",
		url, int(h.InviteTTL.Hours()/24),
	)
	if err := h.Mailer.Send(r.Context(), user.Email, "You're invited to AI Customer Care", body); err != nil {
		// User + token already exist; the link is returned so an admin can
		// still share it manually even if the email failed.
		fmt.Printf("invite email send failed for %s: %v\n", user.Email, err)
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":       user,
		"invite_url": url,
	}, nil)
}

type acceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// AcceptInvite validates the invite token, sets the password and promotes the
// user to VIEWER. Unauthenticated (the invitee has no session yet).
func (h *AuthHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	req.Password = strings.TrimSpace(req.Password)
	if req.Token == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "token are required")
		return
	}
	if len(req.Password) < 8 {
		lib.ResponseJSONError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	upc, err := h.Queries.SelectUpcomingChangeByToken(r.Context(), req.Token)
	if err != nil || upc.Type != db.UpcomingChangesTypeINVITE {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid or expired invite")
		return
	}

	hashedPassword, err := lib.HashPassword(req.Password)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.Queries.SetPasswordAndActivate(r.Context(), db.SetPasswordAndActivateParams{
		HashedPassword: &hashedPassword,
		ID:             upc.UserID,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to activate account")
		return
	}

	if err := h.Queries.RevokeUpcomingChangeByID(r.Context(), upc.ID); err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to finalize invite")
		return
	}

	token, err := h.Signer.Issue(user.ID, string(user.Role))
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":  user,
		"token": token,
	}, nil)
}
