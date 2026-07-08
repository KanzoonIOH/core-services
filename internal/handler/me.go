package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/app/middleware"
	"aic3-service/internal/lib"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MeHandler struct {
	Queries       db.Querier
	Mailer        *lib.Mailer
	ObjectStorage *lib.ObjectStorage
}

func NewMeHandler(conn *pgxpool.Pool, mailer *lib.Mailer, objectStorage *lib.ObjectStorage) *MeHandler {
	return &MeHandler{Queries: db.New(conn), Mailer: mailer, ObjectStorage: objectStorage}
}

func (h *MeHandler) Read(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	user, err := h.Queries.SelectUserById(r.Context(), userID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, user, nil)
}

type updatePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *MeHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	user, err := h.Queries.SelectUserByIdWithPassword(r.Context(), userID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	var req updatePasswordRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.OldPassword = strings.TrimSpace(req.OldPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if req.OldPassword == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "old password are required")
		return
	}
	if req.NewPassword == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "new password are required")
		return
	}
	if req.OldPassword == req.NewPassword {
		lib.ResponseJSONError(w, http.StatusBadRequest, "new password has to be different")
		return
	}

	if user.HashedPassword == nil || !lib.ComparePassword(*user.HashedPassword, req.OldPassword) {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "Password is incorrect")
		return
	}

	hashedPassword, err := lib.HashPassword(req.NewPassword)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	newUser, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		HashedPassword: &hashedPassword,
		ID:             userID,
	})

	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, newUser, nil)
}

type updateDetailsRequest struct {
	Name     string  `json:"name"`
	Username string  `json:"username"`
	Image    *string `json:"image"` // emoji string, or "" to clear; nil leaves unchanged
}

func (h *MeHandler) UpdateDetails(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
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
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.Username == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "username are required")
		return
	}

	user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		Name:     &req.Name,
		Username: &req.Username,
		Image:    req.Image,
		ID:       userID,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, user, nil)
}

// UpdateAvatar uploads a profile picture (multipart field "file") to object
// storage under avatars/ and stores its URL as the user's image.
func (h *MeHandler) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	objectKey := "avatars/" + timestampedObjectFilename(fileHeader.Filename)
	imageURL, err := h.ObjectStorage.Upload(r.Context(), objectKey, file, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to upload avatar")
		return
	}

	user, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
		Image: &imageURL,
		ID:    userID,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, user, nil)
}

type updateEmailRequest struct {
	Email string `json:"new_email"`
}

func (h *MeHandler) UpdateEmailRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		lib.ResponseJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := claims.UserID

	var req updateEmailRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "new email are required")
		return
	}

	user, err := h.Queries.SelectUserById(r.Context(), userID)
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	if user.Email == req.Email {
		lib.ResponseJSONError(w, http.StatusBadRequest, "email not changed")
		return
	}

	changeToken, err := lib.GenerateSecureToken(32)
	if err != nil {
		fmt.Printf("change email request: generate token: %v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	upc, err := h.Queries.InsertUpcomingChange(r.Context(), db.InsertUpcomingChangeParams{
		Token:         changeToken,
		Type:          db.UpcomingChangesTypeEMAIL,
		UpcomingValue: &req.Email,
		UserID:        user.ID,
	})
	if err != nil {
		fmt.Printf("change email request: insert upcoming change: %v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to request changes")
		return
	}

	url := h.Mailer.IssueURL(upc.Token)
	body := fmt.Sprintf(
		"Hi %s,\n\nClick the link below to confirm your email change:\n\n%s\n\nThis link expires in 1 hour.\n\nIf it is not you, please ignore this message.",
		user.Name, url,
	)

	if err := h.Mailer.Send(r.Context(), user.Email, "Confirm Your Email Change", body); err != nil {
		fmt.Printf("change-email: send email: %v\n", err)
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, "email change confirmation has been sent to your email", nil)
}
