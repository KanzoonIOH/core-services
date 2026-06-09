package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeHandler struct {
	Queries db.Querier
}

func NewKnowledgeHandler(conn *pgxpool.Pool) *KnowledgeHandler {
	return &KnowledgeHandler{Queries: db.New(conn)}
}

type createKnowledgeRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SourceType  string  `json:"source_type"`
	SourceUri   *string `json:"source_uri"`
}

func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createKnowledgeRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.SourceType = strings.TrimSpace(req.SourceType)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.SourceType == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "source_type are required")
		return
	}

	// TODO: Insert the knowledge to milvus

	knowledge, err := h.Queries.InsertKnowledge(r.Context(), db.InsertKnowledgeParams{
		Name:        req.Name,
		Description: req.Description,
		SourceType:  req.SourceType,
		SourceUri:   req.SourceUri,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create knowledge")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
}

func (h *KnowledgeHandler) Read(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	sourceType := lib.ParseParamsString(params, "source_type")

	knowledges, err := h.Queries.SelectKnowledges(r.Context(), db.SelectKnowledgesParams{
		SourceType: sourceType,
		Sort:       pagination.Sort,
		Limit:      pagination.Limit,
		Offset:     pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountKnowledges(r.Context(), sourceType)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadByAgentId(w http.ResponseWriter, r *http.Request) {
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	knowledges, err := h.Queries.SelectKnowledgesByAgentId(r.Context(), db.SelectKnowledgesByAgentIdParams{
		AgentID: agent_id,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	totalRow, err := h.Queries.CountKnowledgesByAgentId(r.Context(), agent_id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledges")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledges, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(knowledges), int(totalRow)))
}

func (h *KnowledgeHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	knowledge, err := h.Queries.SelectKnowledgeById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get knowledge")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
}

type updateKnowledgeRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (h *KnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateKnowledgeRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
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
			lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update knowledge")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, knowledge, nil)
}

func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteKnowledge(r.Context(), id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete knowledge")
		return
	}
	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "knowledge not found")
		return
	}

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
