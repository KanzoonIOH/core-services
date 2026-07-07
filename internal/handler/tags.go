package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TagHandler struct {
	Queries db.Querier
}

func NewTagHandler(conn *pgxpool.Pool) *TagHandler {
	return &TagHandler{Queries: db.New(conn)}
}

// Read lists all tags for the tags-management menu and the agent picker.
func (h *TagHandler) Read(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Queries.SelectTags(r.Context())
	if err != nil {
		log.Printf("[core-service][tag-read] db select error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get tags")
		return
	}
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, tags, nil)
}

type updateTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Update renames / recolors a tag. Because agents reference tags by id, the
// change is reflected on every agent automatically.
func (h *TagHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateTagRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Color = strings.TrimSpace(req.Color)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Color == "" {
		req.Color = defaultTagColor(req.Name)
	}

	tag, err := h.Queries.UpdateTag(r.Context(), db.UpdateTagParams{
		Name:  req.Name,
		Color: req.Color,
		ID:    id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "tag not found")
			return
		}
		// Unique violation on name -> friendly 409.
		if strings.Contains(err.Error(), "idx_tags_name_unique") {
			lib.ResponseJSONError(w, http.StatusConflict, "a tag with that name already exists")
			return
		}
		log.Printf("[core-service][tag-update] db update error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update tag")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, tag, nil)
}

// Delete soft-deletes a tag. Agent selects join tags_view (live tags only), so
// a soft-deleted tag stops appearing on every agent automatically.
func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rows, err := h.Queries.SoftDeleteTag(r.Context(), id)
	if err != nil {
		log.Printf("[core-service][tag-delete] db delete error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	if rows == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "tag not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
