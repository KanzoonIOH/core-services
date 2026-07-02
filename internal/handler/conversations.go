package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConversationsHandler serves chat history: the list of conversations and the
// full message thread for one conversation, both from Postgres.
type ConversationsHandler struct {
	Queries db.Querier
}

func NewConversationsHandler(conn *pgxpool.Pool) *ConversationsHandler {
	return &ConversationsHandler{Queries: db.New(conn)}
}

// List handles GET /api/conversations — paginated, most-recent-activity first.
func (h *ConversationsHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := lib.ParsePaginationParams(r.URL.Query())

	rows, err := h.Queries.SelectConversations(r.Context(), db.SelectConversationsParams{
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversations")
		return
	}

	total, err := h.Queries.CountConversations(r.Context())
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversations")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, rows,
		lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(rows), int(total)))
}

// Read handles GET /api/conversations/{id} — conversation meta + full message
// thread (chronological). Used to reopen a conversation and continue it.
func (h *ConversationsHandler) Read(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	conv, err := h.Queries.SelectConversationById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "conversation not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversation")
		return
	}

	messages, err := h.Queries.ListMessagesByConversation(r.Context(), id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"conversation": conv,
		"messages":     messages,
	}, nil)
}
