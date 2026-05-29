package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ApiKeyHandler struct {
	Queries db.Querier
}

func NewApiKeyHandler(conn *pgxpool.Pool) *ApiKeyHandler {
	return &ApiKeyHandler{Queries: db.New(conn)}
}

func generateApiToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "aiac_" + base64.RawURLEncoding.EncodeToString(b), nil
}

type apiKeyCreateRequest struct {
	Name string `json:"name"`
}

func (h *ApiKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req apiKeyCreateRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	token, err := generateApiToken()
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to generate api key token")
		return
	}

	apiKey, err := h.Queries.InsertApiKey(r.Context(), db.InsertApiKeyParams{
		Name:  req.Name,
		Token: token,
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, apiKey, nil)
}

func (h *ApiKeyHandler) Read(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	apiKeys, err := h.Queries.SelectApiKeys(r.Context(), db.SelectApiKeysParams{
		Sort:   pagination.Sort,
		Limit:  pagination.Limit,
		Offset: pagination.Offset,
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get api keys")
		return
	}

	totalRow, err := h.Queries.CountApiKeys(r.Context())
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get api keys")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, apiKeys, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(apiKeys), int(totalRow)))
}

func (h *ApiKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.RevokeApiKey(r.Context(), id)
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "api key not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}
