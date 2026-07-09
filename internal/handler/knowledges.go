package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

type createKnowledgeRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SourceType  string  `json:"source_type"`
	SourceUri   *string `json:"source_uri"`
	IsCrawl     bool    `json:"is_crawl"`
}

func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][knowledge-create] api hit method=%s path=%s", r.Method, r.URL.Path)

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		log.Printf("[core-service][knowledge-create] api error invalid multipart form")
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
		log.Printf("[core-service][knowledge-create] api error status=%d reason=name is required", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.SourceType == "" {
		log.Printf("[core-service][knowledge-create] api error status=%d reason=source_type is required", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "source_type are required")
		return
	}

	// Link knowledges store the URL directly; everything else uploads a file.
	var sourceURI string
	if req.SourceType == sourceTypeLink {
		if req.SourceUri == nil {
			log.Printf("[core-service][knowledge-create] api error status=%d reason=source_uri is required for link", http.StatusBadRequest)
			lib.ResponseJSONError(w, http.StatusBadRequest, "source_uri is required for link knowledge")
			return
		}
		sourceURI = *req.SourceUri
		log.Printf("[core-service][knowledge-create] request params={name:%q source_type:link source_uri:%q is_crawl:%t}", req.Name, sourceURI, req.IsCrawl)
	} else {
		req.IsCrawl = false // crawl is only meaningful for links
		file, fileHeader, err := r.FormFile("file")
		if err != nil {
			log.Printf("[core-service][knowledge-create] api error status=%d reason=file is required", http.StatusBadRequest)
			lib.ResponseJSONError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		log.Printf("[core-service][knowledge-create] request params={name:%q source_type:%q description_present:%t file_name:%q file_size:%d content_type:%q}", req.Name, req.SourceType, req.Description != nil, fileHeader.Filename, fileHeader.Size, fileHeader.Header.Get("Content-Type"))

		objectKey := "knowledges/" + timestampedObjectFilename(fileHeader.Filename)
		log.Printf("[core-service][knowledge-create] object storage upload start key=%q content_type=%q", objectKey, fileHeader.Header.Get("Content-Type"))
		sourceURI, err = h.ObjectStorage.Upload(r.Context(), objectKey, file, fileHeader.Header.Get("Content-Type"))
		if err != nil {
			log.Printf("[core-service][knowledge-create] object storage upload error key=%q error=%v", objectKey, err)
			lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to upload knowledge file")
			return
		}
		log.Printf("[core-service][knowledge-create] object storage upload success key=%q source_uri=%q", objectKey, sourceURI)
	}

	knowledge, err := h.Queries.InsertKnowledge(r.Context(), db.InsertKnowledgeParams{
		Name:        req.Name,
		Description: req.Description,
		SourceType:  req.SourceType,
		SourceUri:   &sourceURI,
		IsCrawl:     req.IsCrawl,
	})
	if err != nil {
		log.Printf("[core-service][knowledge-create] db insert error params={name:%q source_type:%q source_uri:%q} error=%v", req.Name, req.SourceType, sourceURI, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create knowledge")
		return
	}

	log.Printf("[core-service][knowledge-create] api success status=%d response=%s", http.StatusOK, jsonForLog(knowledge))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
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
	log.Printf("[core-service][knowledge-read] api hit method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	sourceType := lib.ParseParamsString(params, "source_type")
	search := lib.ParseParamsString(params, "search")
	log.Printf("[core-service][knowledge-read] request params={search:%v source_type:%v sort:%v limit:%d offset:%d}", search, sourceType, pagination.Sort, pagination.Limit, pagination.Offset)

	knowledges, err := h.Queries.SelectKnowledges(r.Context(), db.SelectKnowledgesParams{
		SourceType: sourceType,
		Search:     search,
		Sort:       pagination.Sort,
		Limit:      pagination.Limit,
		Offset:     pagination.Offset * pagination.Limit,
	})
	if err != nil {
		log.Printf("[core-service][knowledge-read] db select error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountKnowledges(r.Context(), db.CountKnowledgesParams{
		SourceType: sourceType,
		Search:     search,
	})
	if err != nil {
		log.Printf("[core-service][knowledge-read] db count error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	log.Printf("[core-service][knowledge-read] api success status=%d count=%d total=%d", http.StatusOK, len(knowledges), totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadByAgentId(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][knowledge-read-by-agent] api hit method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][knowledge-read-by-agent] api error invalid agent id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	search := lib.ParseParamsString(params, "search")
	log.Printf("[core-service][knowledge-read-by-agent] request params={agent_id:%s search:%v sort:%v limit:%d offset:%d}", agent_id, search, pagination.Sort, pagination.Limit, pagination.Offset)

	knowledges, err := h.Queries.SelectKnowledgesByAgentId(r.Context(), db.SelectKnowledgesByAgentIdParams{
		AgentID: agent_id,
		Search:  search,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		log.Printf("[core-service][knowledge-read-by-agent] db select error agent_id=%s error=%v", agent_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountKnowledgesByAgentId(r.Context(), db.CountKnowledgesByAgentIdParams{
		AgentID: agent_id,
		Search:  search,
	})
	if err != nil {
		log.Printf("[core-service][knowledge-read-by-agent] db count error agent_id=%s error=%v", agent_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	log.Printf("[core-service][knowledge-read-by-agent] api success status=%d agent_id=%s count=%d total=%d", http.StatusOK, agent_id, len(knowledges), totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadAllByAgentId(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][knowledge-read-all-by-agent] api hit method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][knowledge-read-all-by-agent] api error invalid agent id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	search := lib.ParseParamsString(params, "search")
	log.Printf("[core-service][knowledge-read-all-by-agent] request params={agent_id:%s search:%v sort:%v limit:%d offset:%d}", agent_id, search, pagination.Sort, pagination.Limit, pagination.Offset)

	knowledges, err := h.Queries.SelectKnowledgesWithAgentStatus(r.Context(), db.SelectKnowledgesWithAgentStatusParams{
		AgentID: agent_id,
		Search:  search,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		log.Printf("[core-service][knowledge-read-all-by-agent] db select error agent_id=%s error=%v", agent_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountAllKnowledges(r.Context(), search)
	if err != nil {
		log.Printf("[core-service][knowledge-read-all-by-agent] db count error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	log.Printf("[core-service][knowledge-read-all-by-agent] api success status=%d agent_id=%s count=%d total=%d", http.StatusOK, agent_id, len(knowledges), totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][knowledge-read-by-id] api hit method=%s path=%s", r.Method, r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][knowledge-read-by-id] api error invalid id")
		return
	}
	log.Printf("[core-service][knowledge-read-by-id] request params={id:%s}", id)

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[core-service][knowledge-read-by-id] api error status=%d reason=knowledge not found id=%s", http.StatusNotFound, id)
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}

		log.Printf("[core-service][knowledge-read-by-id] db select error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledge")
		return
	}

	log.Printf("[core-service][knowledge-read-by-id] api success status=%d response=%s", http.StatusOK, jsonForLog(knowledge))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
}

type updateKnowledgeRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (h *KnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][knowledge-update] api hit method=%s path=%s", r.Method, r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][knowledge-update] api error invalid id")
		return
	}

	var req updateKnowledgeRequest
	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][knowledge-update] api error invalid JSON body id=%s", id)
		return
	}
	log.Printf("[core-service][knowledge-update] request params={id:%s body:%s}", id, jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		log.Printf("[core-service][knowledge-update] api error status=%d reason=name is required id=%s", http.StatusBadRequest, id)
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
			log.Printf("[core-service][knowledge-update] api error status=%d reason=knowledge not found id=%s", http.StatusNotFound, id)
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}

		log.Printf("[core-service][knowledge-update] db update error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update knowledge")
		return
	}

	log.Printf("[core-service][knowledge-update] api success status=%d response=%s", http.StatusOK, jsonForLog(knowledge))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
}

func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][knowledge-delete] api hit method=%s path=%s", r.Method, r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][knowledge-delete] api error invalid id")
		return
	}
	log.Printf("[core-service][knowledge-delete] request params={id:%s}", id)

	rowsAffected, err := h.Queries.SoftDeleteKnowledge(r.Context(), id)
	if err != nil {
		log.Printf("[core-service][knowledge-delete] db soft-delete error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete knowledge")
		return
	}
	if rowsAffected == 0 {
		log.Printf("[core-service][knowledge-delete] api error status=%d reason=knowledge not found id=%s", http.StatusNotFound, id)
		lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
		return
	}

	log.Printf("[core-service][knowledge-delete] api success status=%d rows_affected=%d", http.StatusNoContent, rowsAffected)
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
