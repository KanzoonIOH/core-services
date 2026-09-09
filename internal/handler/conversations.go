package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"errors"
	"net/http"

	"github.com/google/uuid"
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
// ?mine=true scopes the list to the caller's own conversations (the viewer app);
// without it the full history is returned (the admin console).
func (h *ConversationsHandler) List(w http.ResponseWriter, r *http.Request) {
	pagination := lib.ParsePaginationParams(r.URL.Query())

	userID, _, ok := currentUser(w, r)
	if !ok {
		return
	}
	var owner *uuid.UUID
	if r.URL.Query().Get("mine") == "true" {
		owner = &userID
	}

	rows, err := h.Queries.SelectConversations(r.Context(), db.SelectConversationsParams{
		UserID: owner,
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversations")
		return
	}

	total, err := h.Queries.CountConversations(r.Context(), owner)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversations")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, rows,
		lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(rows), int(total)))
}

// canViewConversation reports whether a caller may read/delete a conversation.
// Ownerless rows (API-key traffic, pre-user_id history) stay visible to
// everyone, same as before the column existed.
func canViewConversation(owner *uuid.UUID, userID uuid.UUID, role string) bool {
	return owner == nil || *owner == userID || isAdmin(role)
}

// loadOwned fetches a conversation and hides it (404) from a non-admin who is
// not its owner.
func (h *ConversationsHandler) loadOwned(w http.ResponseWriter, r *http.Request, id uuid.UUID) (db.SelectConversationByIdRow, bool) {
	userID, role, ok := currentUser(w, r)
	if !ok {
		return db.SelectConversationByIdRow{}, false
	}

	conv, err := h.Queries.SelectConversationById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "conversation not found")
			return conv, false
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get conversation")
		return conv, false
	}

	if !canViewConversation(conv.UserID, userID, role) {
		lib.ResponseJSONError(w, http.StatusNotFound, "conversation not found")
		return conv, false
	}
	return conv, true
}

// Read handles GET /api/conversations/{id} — conversation meta + full message
// thread (chronological). Used to reopen a conversation and continue it.
func (h *ConversationsHandler) Read(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	conv, ok := h.loadOwned(w, r, id)
	if !ok {
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

// Delete handles DELETE /api/conversations/{id} — removes the conversation and
// its messages. Only the owner (or an admin) may delete.
func (h *ConversationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	if _, ok := h.loadOwned(w, r, id); !ok {
		return
	}

	// messages has no FK cascade (see migration), so delete them explicitly.
	if err := h.Queries.DeleteMessagesByConversation(r.Context(), id); err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete conversation")
		return
	}
	if err := h.Queries.DeleteConversation(r.Context(), id); err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete conversation")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, nil, nil)
}
