package handler

import (
	"aic3-service/internal/lib"
	"fmt"
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
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to upload image")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]string{"url": url}, nil)
}
