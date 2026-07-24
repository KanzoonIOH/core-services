package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeHandler struct {
	Queries       db.Querier
	ObjectStorage *lib.ObjectStorage
}

func NewKnowledgeHandler(conn *pgxpool.Pool, objectStorage *lib.ObjectStorage) *KnowledgeHandler {
	return &KnowledgeHandler{Queries: db.New(conn), ObjectStorage: objectStorage}
}

// sourceTypeLink is the source_type for URL-based knowledges (no file upload).
const sourceTypeLink = "web"

// syncKnowledgeTags mirrors syncAgentTags for knowledges.
func (h *KnowledgeHandler) syncKnowledgeTags(ctx context.Context, knowledgeID uuid.UUID, names []string) error {
	if err := h.Queries.DeleteKnowledgeTags(ctx, knowledgeID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		tag, err := h.Queries.UpsertTagByName(ctx, db.UpsertTagByNameParams{
			Name:  name,
			Color: defaultTagColor(name),
		})
		if err != nil {
			return err
		}
		if err := h.Queries.InsertKnowledgeTag(ctx, db.InsertKnowledgeTagParams{
			KnowledgeID: knowledgeID,
			TagID:       tag.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// formTags reads tags from a multipart form: repeated `tags` fields, or a single
// comma-separated `tags` value. Blank entries are dropped by syncKnowledgeTags.
func formTags(r *http.Request) []string {
	vals := r.Form["tags"]
	if len(vals) == 1 && strings.Contains(vals[0], ",") {
		return strings.Split(vals[0], ",")
	}
	return vals
}

type createKnowledgeRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SourceType  string  `json:"source_type"`
	SourceUri   *string `json:"source_uri"`
	IsCrawl     bool    `json:"is_crawl"`
}

func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: create api hit", "method", r.Method, "path", r.URL.Path)

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		slog.ErrorContext(r.Context(), "knowledges: create invalid multipart form")
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	req := createKnowledgeRequest{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: formStringPtr(r.FormValue("description")),
		SourceType:  strings.TrimSpace(r.FormValue("source_type")),
		SourceUri:   formStringPtr(r.FormValue("source_uri")),
		IsCrawl:     r.FormValue("is_crawl") == "true",
	}

	if req.Name == "" {
		slog.WarnContext(r.Context(), "knowledges: create name is required", "status", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.SourceType == "" {
		slog.WarnContext(r.Context(), "knowledges: create source_type is required", "status", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "source_type are required")
		return
	}

	// Link knowledges store the URL directly; everything else uploads a file.
	var sourceURI string
	if req.SourceType == sourceTypeLink {
		if req.SourceUri == nil {
			slog.WarnContext(r.Context(), "knowledges: create source_uri is required for link", "status", http.StatusBadRequest)
			lib.ResponseJSONError(w, http.StatusBadRequest, "source_uri is required for link knowledge")
			return
		}
		sourceURI = *req.SourceUri
		slog.InfoContext(r.Context(), "knowledges: create request", "name", req.Name, "source_type", "link", "source_uri", sourceURI, "is_crawl", req.IsCrawl)
	} else {
		req.IsCrawl = false // crawl is only meaningful for links
		file, fileHeader, err := r.FormFile("file")
		if err != nil {
			slog.WarnContext(r.Context(), "knowledges: create file is required", "status", http.StatusBadRequest)
			lib.ResponseJSONError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		slog.InfoContext(r.Context(), "knowledges: create request", "name", req.Name, "source_type", req.SourceType, "description_present", req.Description != nil, "file_name", fileHeader.Filename, "file_size", fileHeader.Size, "content_type", fileHeader.Header.Get("Content-Type"))

		objectKey := "knowledges/" + timestampedObjectFilename(fileHeader.Filename)
		slog.InfoContext(r.Context(), "knowledges: create object storage upload start", "key", objectKey, "content_type", fileHeader.Header.Get("Content-Type"))
		sourceURI, err = h.ObjectStorage.Upload(r.Context(), objectKey, file, fileHeader.Header.Get("Content-Type"))
		if err != nil {
			slog.ErrorContext(r.Context(), "knowledges: create object storage upload failed", "key", objectKey, "error", err)
			lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to upload knowledge file")
			return
		}
		slog.InfoContext(r.Context(), "knowledges: create object storage upload success", "key", objectKey, "source_uri", sourceURI)
	}

	knowledge, err := h.Queries.InsertKnowledge(r.Context(), db.InsertKnowledgeParams{
		Name:        req.Name,
		Description: req.Description,
		SourceType:  req.SourceType,
		SourceUri:   &sourceURI,
		IsCrawl:     req.IsCrawl,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: create db insert failed", "name", req.Name, "source_type", req.SourceType, "source_uri", sourceURI, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create knowledge")
		return
	}

	if err := h.syncKnowledgeTags(r.Context(), knowledge.ID, formTags(r)); err != nil {
		slog.ErrorContext(r.Context(), "knowledges: create tag sync failed", "knowledge_id", knowledge.ID, "error", err)
	}

	full, err := h.Queries.SelectKnowledgeById(r.Context(), knowledge.ID)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create knowledge")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: create success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

func formStringPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeObjectFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	filename = unsafeFilenameChars.ReplaceAllString(filename, "-")
	filename = strings.Trim(filename, ".-")
	if filename == "" {
		return "upload"
	}
	return filename
}

func timestampedObjectFilename(filename string) string {
	filename = sanitizeObjectFilename(filename)
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	return fmt.Sprintf("%s_%d%s", name, time.Now().UnixMilli(), ext)
}

func (h *KnowledgeHandler) Read(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: read api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	sourceType := lib.ParseParamsString(params, "source_type")
	search := lib.ParseParamsString(params, "search")
	tagID := lib.ParseParamsUUID(params, "tag_id")
	slog.InfoContext(r.Context(), "knowledges: read request", "search", search, "source_type", sourceType, "tag_id", tagID, "sort", pagination.Sort, "limit", pagination.Limit, "offset", pagination.Offset)

	knowledges, err := h.Queries.SelectKnowledges(r.Context(), db.SelectKnowledgesParams{
		SourceType: sourceType,
		Search:     search,
		TagID:      tagID,
		Sort:       pagination.Sort,
		Limit:      pagination.Limit,
		Offset:     pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: read db select failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountKnowledges(r.Context(), db.CountKnowledgesParams{
		SourceType: sourceType,
		Search:     search,
		TagID:      tagID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: read db count failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: read success", "status", http.StatusOK, "count", len(knowledges), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadByAgentId(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: read by agent api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "knowledges: read by agent invalid agent id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	search := lib.ParseParamsString(params, "search")
	slog.InfoContext(r.Context(), "knowledges: read by agent request", "agent_id", agent_id, "search", search, "sort", pagination.Sort, "limit", pagination.Limit, "offset", pagination.Offset)

	knowledges, err := h.Queries.SelectKnowledgesByAgentId(r.Context(), db.SelectKnowledgesByAgentIdParams{
		AgentID: agent_id,
		Search:  search,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: read by agent db select failed", "agent_id", agent_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountKnowledgesByAgentId(r.Context(), db.CountKnowledgesByAgentIdParams{
		AgentID: agent_id,
		Search:  search,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: read by agent db count failed", "agent_id", agent_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: read by agent success", "status", http.StatusOK, "agent_id", agent_id, "count", len(knowledges), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadAllByAgentId(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: read all by agent api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "knowledges: read all by agent invalid agent id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	search := lib.ParseParamsString(params, "search")
	slog.InfoContext(r.Context(), "knowledges: read all by agent request", "agent_id", agent_id, "search", search, "sort", pagination.Sort, "limit", pagination.Limit, "offset", pagination.Offset)

	knowledges, err := h.Queries.SelectKnowledgesWithAgentStatus(r.Context(), db.SelectKnowledgesWithAgentStatusParams{
		AgentID: agent_id,
		Search:  search,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: read all by agent db select failed", "agent_id", agent_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountAllKnowledges(r.Context(), search)
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: read all by agent db count failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: read all by agent success", "status", http.StatusOK, "agent_id", agent_id, "count", len(knowledges), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: read by id api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "knowledges: read by id invalid id")
		return
	}
	slog.InfoContext(r.Context(), "knowledges: read by id request", "knowledge_id", id)

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(r.Context(), "knowledges: read by id not found", "status", http.StatusNotFound, "knowledge_id", id)
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}

		slog.ErrorContext(r.Context(), "knowledges: read by id db select failed", "knowledge_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledge")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: read by id success", "status", http.StatusOK, "response", jsonForLog(knowledge))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
}

type updateKnowledgeRequest struct {
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Tags        []string `json:"tags"`
}

func (h *KnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: update api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "knowledges: update invalid id")
		return
	}

	var req updateKnowledgeRequest
	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "knowledges: update invalid JSON body", "knowledge_id", id)
		return
	}
	slog.InfoContext(r.Context(), "knowledges: update request", "knowledge_id", id, "body", jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		slog.WarnContext(r.Context(), "knowledges: update name is required", "status", http.StatusBadRequest, "knowledge_id", id)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}

	knowledge, err := h.Queries.UpdateKnowledge(r.Context(), db.UpdateKnowledgeParams{
		Name:        req.Name,
		Description: req.Description,
		ID:          id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(r.Context(), "knowledges: update knowledge not found", "status", http.StatusNotFound, "knowledge_id", id)
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}

		slog.ErrorContext(r.Context(), "knowledges: update db update failed", "knowledge_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update knowledge")
		return
	}

	if err := h.syncKnowledgeTags(r.Context(), knowledge.ID, req.Tags); err != nil {
		slog.ErrorContext(r.Context(), "knowledges: update tag sync failed", "knowledge_id", knowledge.ID, "error", err)
	}

	full, err := h.Queries.SelectKnowledgeById(r.Context(), knowledge.ID)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update knowledge")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: update success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "knowledges: delete api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "knowledges: delete invalid id")
		return
	}
	slog.InfoContext(r.Context(), "knowledges: delete request", "knowledge_id", id)

	rowsAffected, err := h.Queries.SoftDeleteKnowledge(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "knowledges: delete db soft-delete failed", "knowledge_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete knowledge")
		return
	}
	if rowsAffected == 0 {
		slog.WarnContext(r.Context(), "knowledges: delete knowledge not found", "status", http.StatusNotFound, "knowledge_id", id)
		lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
		return
	}

	slog.InfoContext(r.Context(), "knowledges: delete success", "status", http.StatusNoContent, "rows_affected", rowsAffected)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

// func (h *KnowledgeHandler) ReadIdAgents(w http.ResponseWriter, r *http.Request) {
// 	id, ok := lib.ParseID(w, r, "id")
// 	if !ok {
// 		return
// 	}

// 	agents, err := h.Queries.GetKnowledgeAgents(r.Context(), id)
// 	if err != nil {
// 		lib.ResponseJSON(w, http.StatusInternalServerError, "failed to get knowledge agents")
// 		return
// 	}

// 	lib.ResponseJSON(w, http.StatusOK, agents)
// }
