package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	Queries    db.Querier
	Signer     *lib.JWTSigner
	Mailer     *lib.Mailer
	InviteTTL  time.Duration
	RefreshTTL time.Duration
	Kafka      *lib.KafkaProducer
}

func NewAuthHandler(conn *pgxpool.Pool, signer *lib.JWTSigner, mailer *lib.Mailer, inviteTTL, refreshTTL time.Duration, kafka *lib.KafkaProducer) *AuthHandler {
	return &AuthHandler{
		Queries:    db.New(conn),
		Signer:     signer,
		Mailer:     mailer,
		InviteTTL:  inviteTTL,
		RefreshTTL: refreshTTL,
		Kafka:      kafka,
	}
}

// issueSession mints a short-lived access JWT and a long-lived refresh token,
// persisting the refresh token (hashed) as a DB-backed session row. Returns both
// tokens for the response body. The access token stays stateless; the refresh
// token is the revocable server-side session.
func (h *AuthHandler) issueSession(r *http.Request, userID uuid.UUID, role string) (access, refresh string, err error) {
	access, err = h.Signer.Issue(userID, role)
	if err != nil {
		return "", "", err
	}
	refresh, err = lib.GenerateSecureToken(32)
	if err != nil {
		return "", "", err
	}
	_, err = h.Queries.InsertRefreshToken(r.Context(), db.InsertRefreshTokenParams{
		UserID:    userID,
		TokenHash: lib.HashToken(refresh),
		UserAgent: r.UserAgent(),
		Ip:        sessionIP(r),
		ExpiresAt: time.Now().UTC().Add(h.RefreshTTL),
	})
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// sessionIP is a best-effort remote address for session bookkeeping. ponytail:
// trusts X-Forwarded-For's first hop; tighten only if the proxy chain is
// untrusted.
func sessionIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}

// auditAuth records a login/logout event via the same Kafka->ClickHouse audit
// pipeline as write actions. Fire-and-forget: a broker hiccup must never break
// auth. ponytail: reuses the existing AuditEvent/audit_logs schema, no new table.
func (h *AuthHandler) auditAuth(r *http.Request, userID, role, action string) {
	if h.Kafka == nil {
		return
	}
	_ = h.Kafka.Publish(context.Background(), middleware.TopicAudit, middleware.AuditEvent{
		Time:       time.Now().UTC(),
		UserID:     userID,
		Role:       role,
		AuthMethod: middleware.AuthMethodJWT,
		Action:     action,
		Menu:       "auth",
		Method:     r.Method,
		Path:       r.URL.Path,
		Status:     http.StatusOK,
	})
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
		slog.ErrorContext(r.Context(), "register: insert user", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to register user")
		return
	}

	access, refresh, err := h.issueSession(r, user.ID, string(user.Role))
	if err != nil {
		slog.ErrorContext(r.Context(), "register: issue session", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	h.auditAuth(r, user.ID.String(), string(user.Role), "REGISTER")

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":          user,
		"token":         access,
		"refresh_token": refresh,
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

	// Same 401 for "no such user" and "wrong password" — never reveal which.
	user, err := h.Queries.SelectUserByLoginIdWithPassword(r.Context(), req.LoginID)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if user.HashedPassword == nil || !lib.ComparePassword(*user.HashedPassword, req.Password) {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	access, refresh, err := h.issueSession(r, user.ID, string(user.Role))
	if err != nil {
		slog.ErrorContext(r.Context(), "login: issue session", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	h.auditAuth(r, user.ID.String(), string(user.Role), "LOGIN")

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":          lib.ToUserResponse(user),
		"token":         access,
		"refresh_token": refresh,
	}, nil)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh rotates a refresh token: it validates the presented token against the
// DB-backed session, re-reads the CURRENT user role/deleted state, then revokes
// the old token and issues a fresh access+refresh pair. A suspended/soft-deleted
// user's session stops here — so status changes take effect within one access
// TTL. Rotation (revoke-old, issue-new) also detects token theft/replay.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	hash := lib.HashToken(req.RefreshToken)
	sess, err := h.Queries.SelectActiveRefreshToken(r.Context(), hash)
	if err != nil {
		// Not found / expired / revoked — all indistinguishable to the caller.
		lib.ResponseJSONError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Live user re-check: a soft-deleted user (deactivated) can no longer refresh.
	if sess.DeletedAt != nil {
		_ = h.Queries.RevokeAllUserRefreshTokens(r.Context(), sess.UserID)
		lib.ResponseJSONError(w, http.StatusUnauthorized, "account is no longer active")
		return
	}

	// Rotate: kill the presented token, mint a new session.
	if err := h.Queries.RevokeRefreshTokenByHash(r.Context(), hash); err != nil {
		slog.ErrorContext(r.Context(), "refresh: revoke old token", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to refresh")
		return
	}

	access, refresh, err := h.issueSession(r, sess.UserID, string(sess.Role))
	if err != nil {
		slog.ErrorContext(r.Context(), "refresh: issue session", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to refresh")
		return
	}

	h.auditAuth(r, sess.UserID.String(), string(sess.Role), "REFRESH")

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"token":         access,
		"refresh_token": refresh,
	}, nil)
}

// Logout revokes the caller's refresh-token session so it can no longer be
// refreshed. The access JWT itself stays valid until it expires (bounded by the
// short access TTL); the client discards it. ponytail: no access-token blacklist
// — the short TTL is the ceiling, add a blacklist only if instant kill is needed.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Decode directly (not ParseJSONBody) so an empty body doesn't 400 — logout
	// must always succeed and clear whatever it can.
	var req refreshRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if rt := strings.TrimSpace(req.RefreshToken); rt != "" {
		_ = h.Queries.RevokeRefreshTokenByHash(r.Context(), lib.HashToken(rt))
	}
	if claims, ok := middleware.ClaimsFromContext(r.Context()); ok {
		h.auditAuth(r, claims.UserID.String(), claims.Role, "LOGOUT")
	}
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{"ok": true}, nil)
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
		slog.ErrorContext(r.Context(), "forgot-password: generate token", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	upc, err := h.Queries.InsertUpcomingChange(r.Context(), db.InsertUpcomingChangeParams{
		Token:  changeToken,
		Type:   db.UpcomingChangesTypeFORGOTPASSWORD,
		UserID: user.ID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "forgot-password: insert upcoming change", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	url := h.Mailer.IssuePathURL(resetPasswordPath, upc.Token)
	body := fmt.Sprintf(
		"Hi %s,\n\nClick the link below to reset your password:\n\n%s\n\nThis link expires in 1 hour.\n\nIf it is not you, please ignore this message.",
		user.Name, url,
	)

	if err := h.Mailer.Send(r.Context(), user.Email, "Confirm Your Forgot Password Request", body); err != nil {
		slog.ErrorContext(r.Context(), "forgot-password: send email", "error", err)
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
		slog.ErrorContext(r.Context(), "invite: send email", "email", user.Email, "error", err)
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

	access, refresh, err := h.issueSession(r, user.ID, string(user.Role))
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	h.auditAuth(r, user.ID.String(), string(user.Role), "ACCEPT_INVITE")

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"user":          user,
		"token":         access,
		"refresh_token": refresh,
	}, nil)
}
