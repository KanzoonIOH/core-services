package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DropdownHandler struct {
	Queries db.Querier
}

func NewDropdownHandler(conn *pgxpool.Pool) *DropdownHandler {
	return &DropdownHandler{Queries: db.New(conn)}
}

func (h *DropdownHandler) ReadAgents(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	knowledgeID, err := uuid.Parse(params.Get("knowledge_id"))
	if err != nil {
		lib.ResponseJSONError(w, http.StatusBadRequest, "knowledge_id are required")
		return
	}

	search := lib.ParseParamsString(params, "search")

	agents, err := h.Queries.SelectDropdownAgents(r.Context(), db.SelectDropdownAgentsParams{
		KnowledgeID: knowledgeID,
		Search:      search,
	})
	if err != nil {
		fmt.Printf("%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, nil)
}
