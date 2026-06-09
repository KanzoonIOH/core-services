package handler

import (
	db "aiac-service/db/postgres/sqlc"
	"aiac-service/internal/lib"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type McpHandler struct {
	Queries db.Querier
}

func NewMcpHandler(conn *pgxpool.Pool) *McpHandler {
	return &McpHandler{Queries: db.New(conn)}
}

type mcpCreateRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Uri         string  `json:"uri"`
}

func (h *McpHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req mcpCreateRequest

	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Uri = strings.TrimSpace(req.Uri)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.Uri == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "uri are required")
		return
	}

	mcp, err := h.Queries.InsertMcp(r.Context(), db.InsertMcpParams{
		Name:        req.Name,
		Description: req.Description,
		Uri:         req.Uri,
	})
	if err != nil {
		fmt.Printf("%v\n", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create mcp")
		return
	}

	tools, discoverErr := h.syncTools(r.Context(), mcp.ID, mcp.Uri)
	if discoverErr != nil {
		fmt.Printf("mcp tool discovery failed for %s: %v\n", mcp.ID, discoverErr)
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]any{
		"mcp":   mcp,
		"tools": tools,
	}, nil)
}

func (h *McpHandler) syncTools(ctx context.Context, mcpID uuid.UUID, uri string) ([]db.InsertMcpToolRow, error) {
	discovered, err := lib.DiscoverMcpTools(ctx, uri)
	if err != nil {
		return nil, err
	}

	if _, err := h.Queries.SoftDeleteMcpToolsByMcpId(ctx, mcpID); err != nil {
		return nil, fmt.Errorf("clear existing mcp tools: %w", err)
	}

	tools := make([]db.InsertMcpToolRow, 0, len(discovered))
	for _, t := range discovered {
		var description *string
		if t.Description != "" {
			d := t.Description
			description = &d
		}

		tool, err := h.Queries.InsertMcpTool(ctx, db.InsertMcpToolParams{
			McpID:       mcpID,
			Name:        t.Name,
			Description: description,
			TargetUri:   uri,
			InputSchema: t.InputSchema,
		})
		if err != nil {
			return nil, fmt.Errorf("insert mcp tool %q: %w", t.Name, err)
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

func (h *McpHandler) Read(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	mcps, err := h.Queries.SelectMcps(r.Context(), db.SelectMcpsParams{
		Sort:   pagination.Sort,
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcps")
		return
	}

	totalRow, err := h.Queries.CountMcps(r.Context())
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcps")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, mcps, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(mcps), int(totalRow)))
}

func (h *McpHandler) ReadByAgentId(w http.ResponseWriter, r *http.Request) {
	agent_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)

	mcps, err := h.Queries.SelectMcpsByAgentId(r.Context(), db.SelectMcpsByAgentIdParams{
		AgentID: agent_id,
		Sort:    pagination.Sort,
		Limit:   pagination.Limit,
		Offset:  pagination.Offset * pagination.Limit,
	})
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcps")
		return
	}

	totalRow, err := h.Queries.CountMcpsByAgentId(r.Context(), agent_id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcps")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, mcps, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(mcps), int(totalRow)))
}

func (h *McpHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	mcp, err := h.Queries.SelectMcpById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "mcp not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcp")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, mcp, nil)
}

type mcpUpdateRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (h *McpHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req mcpUpdateRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}

	mcp, err := h.Queries.UpdateMcp(r.Context(), db.UpdateMcpParams{
		Name:        req.Name,
		Description: req.Description,
		ID:          id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "mcp not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update mcp")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, mcp, nil)
}

func (h *McpHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	rowsAffected, err := h.Queries.SoftDeleteMcp(r.Context(), id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete mcp")
		return
	}
	if rowsAffected == 0 {
		lib.ResponseJSONError(w, http.StatusNotFound, "mcp not found")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

func (h *McpHandler) ReadTools(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	if _, err := h.Queries.SelectMcpById(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "mcp not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcp")
		return
	}

	tools, err := h.Queries.SelectMcpToolsByMcpId(r.Context(), id)
	if err != nil {
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcp tools")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, tools, nil)
}

// RefreshTools reconnects to the MCP server and re-syncs its tools.
func (h *McpHandler) RefreshTools(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	mcp, err := h.Queries.SelectMcpById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "mcp not found")
			return
		}
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get mcp")
		return
	}

	tools, err := h.syncTools(r.Context(), mcp.ID, mcp.Uri)
	if err != nil {
		fmt.Printf("mcp tool refresh failed for %s: %v\n", mcp.ID, err)
		lib.ResponseJSONError(w, http.StatusBadGateway, "failed to discover mcp tools")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, tools, nil)
}
