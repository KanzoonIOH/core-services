package handler

import (
	"aic3-service/internal/lib"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UploadHandler struct {
	ObjectStorage *lib.ObjectStorage
}

func NewUploadHandler(_ *pgxpool.Pool, objectStorage *lib.ObjectStorage) *UploadHandler {
	return &UploadHandler{ObjectStorage: objectStorage}
}

// Image uploads a picture (multipart field "file") under avatars/ and returns
// its public URL. Used by the agent form for custom agent pictures; emojis are
// stored as plain strings and never hit this endpoint.
func (h *UploadHandler) Image(w http.ResponseWriter, r *http.Request) {
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
	url, err := h.ObjectStorage.Upload(r.Context(), objectKey, file, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		slog.ErrorContext(r.Context(), "uploads: image upload failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to upload image")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]string{"url": url}, nil)
}

// maxChatDocumentBytes caps a single chat attachment. Anything bigger belongs
// in a Knowledge, not a chat turn.
const maxChatDocumentBytes = 25 << 20 // 25 MiB

// Document uploads a chat attachment (multipart field "file") under
// chat-documents/ and returns the descriptor the chat UI puts in the outbound
// "attachments" array. Unlike a Knowledge upload this is not indexed or
// persisted anywhere except the message row.
func (h *UploadHandler) Document(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxChatDocumentBytes)
	if err := r.ParseMultipartForm(maxChatDocumentBytes); err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "file too large or invalid multipart form")
		return
	}
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	objectKey := "chat-documents/" + timestampedObjectFilename(fileHeader.Filename)
	url, err := h.ObjectStorage.Upload(r.Context(), objectKey, file, contentType)
	if err != nil {
		slog.ErrorContext(r.Context(), "uploads: document upload failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to upload document")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"name":         sanitizeObjectFilename(fileHeader.Filename),
		"url":          url,
		"content_type": contentType,
		"size":         fileHeader.Size,
	}, nil)
}
