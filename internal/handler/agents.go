package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BodyField is one extra field injected into the outbound webhook JSON body.
// type "static": value is sent as-is. type "dynamic": the caller must supply
// the value per request under Key (enforced in webhooks.go).
type BodyField struct {
	Key   string `json:"key"`
	Type  string `json:"type"` // "static" | "dynamic"
	Value string `json:"value"`
}

// parseWebhookFields decodes a stored webhook field array (body or header)
// back into typed structs. Returns nil on empty/invalid.
func parseWebhookFields(raw json.RawMessage) []BodyField {
	if len(raw) == 0 {
		return nil
	}
	var fields []BodyField
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}

// storedAttachments picks what to persist on the message row. Chat clients
// send attachments twice: "attachments" as plain URL strings (what most agents
// consume) and "attachments_details" as {name,url,content_type,size} objects.
// Prefer the detailed form so the transcript can render names and sizes.
func storedAttachments(bodyMap map[string]any) any {
	if v, ok := bodyMap["attachments_details"]; ok && v != nil {
		return v
	}
	return bodyMap["attachments"]
}

// popCallerHeaders pulls the reserved "headers" object out of an incoming chat
// body. It carries per-request values for "dynamic" header fields and must not
// be forwarded in the body.
func popCallerHeaders(bodyMap map[string]any) map[string]string {
	out := map[string]string{}
	raw, ok := bodyMap["headers"]
	if !ok {
		return out
	}
	if m, ok := raw.(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	delete(bodyMap, "headers")
	return out
}

// applyWebhookBodyFields injects the configured body fields into bodyMap.
// Static fields set a fixed value; dynamic fields must already be supplied by
// the caller. Returns a non-empty message when a dynamic field is missing.
func applyWebhookBodyFields(bodyMap map[string]any, raw json.RawMessage) string {
	for _, f := range parseWebhookFields(raw) {
		if f.Key == "" {
			continue
		}
		if f.Type == "dynamic" {
			v, ok := bodyMap[f.Key]
			if !ok || v == nil || v == "" {
				return fmt.Sprintf("missing required field %q", f.Key)
			}
			continue // keep the caller-supplied value as-is
		}
		bodyMap[f.Key] = f.Value
	}
	return ""
}

// resolveWebhookHeaders turns the configured header fields into outbound HTTP
// headers. Static uses the fixed value; dynamic must be supplied by the caller
// under body.headers. No fields = open target, no headers added. Returns a
// non-empty message when a dynamic header is missing.
func resolveWebhookHeaders(raw json.RawMessage, caller map[string]string) (map[string]string, string) {
	out := map[string]string{}
	for _, f := range parseWebhookFields(raw) {
		if f.Key == "" {
			continue
		}
		if f.Type == "dynamic" {
			v := caller[f.Key]
			if v == "" {
				return nil, fmt.Sprintf("missing required header %q", f.Key)
			}
			out[f.Key] = v
			continue
		}
		out[f.Key] = f.Value
	}
	return out, ""
}

// marshalBodyFields cleans the incoming field list (drops blank keys, defaults
// type to static) and returns JSONB. Always a valid JSON array.
func marshalBodyFields(fields []BodyField) json.RawMessage {
	out := make([]BodyField, 0, len(fields))
	for _, f := range fields {
		f.Key = strings.TrimSpace(f.Key)
		if f.Key == "" {
			continue
		}
		if f.Type != "dynamic" {
			f.Type = "static"
		}
		out = append(out, f)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// trimNonEmpty trims each entry and drops blanks. Returns a non-nil empty
// slice for "allow all".
func trimNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// On PATCH, a JSON key the caller left out unmarshals to a nil slice while an
// explicit [] unmarshals to an empty non-nil slice. The *OnUpdate helpers keep
// that distinction: nil -> NULL -> the SQL COALESCE keeps the stored value,
// empty -> clear. Create must never send NULL (columns are NOT NULL), so it
// keeps using the plain helpers.
func marshalBodyFieldsOnUpdate(fields []BodyField) json.RawMessage {
	if fields == nil {
		return nil
	}
	return marshalBodyFields(fields)
}

func trimNonEmptyOnUpdate(in []string) []string {
	if in == nil {
		return nil
	}
	return trimNonEmpty(in)
}

func parseAllowedIPsOnUpdate(in []string) ([]netip.Addr, error) {
	if in == nil {
		return nil, nil
	}
	return parseAllowedIPs(in)
}

func trimStringOnUpdate(in *string) *string {
	if in == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*in)
	return &trimmed
}

// tagPalette is the fixed set of default colors a newly typed tag can get.
// Deterministic pick by name hash, so the same tag name always starts the same
// color until the user recolors it in the Tags menu.
var tagPalette = []string{
	"#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6",
	"#3b82f6", "#6366f1", "#a855f7", "#ec4899", "#64748b",
}

func defaultTagColor(name string) string {
	var sum uint32
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		sum = sum*31 + uint32(r)
	}
	return tagPalette[sum%uint32(len(tagPalette))]
}

// syncAgentTags upserts each tag name to a tag row, then replaces the agent's
// tag links to exactly that set. Blank names are dropped; dupes collapse.
func (h *AgentHandler) syncAgentTags(ctx context.Context, agentID uuid.UUID, names []string) error {
	if err := h.Queries.DeleteAgentTags(ctx, agentID); err != nil {
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
		if err := h.Queries.InsertAgentTag(ctx, db.InsertAgentTagParams{
			AgentID: agentID,
			TagID:   tag.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// parseAllowedIPs converts a list of IP strings into netip.Addr.
// Empty/nil means "allow all". Invalid entries are rejected.
func parseAllowedIPs(in []string) ([]netip.Addr, error) {
	out := make([]netip.Addr, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid ip %q", s)
		}
		out = append(out, addr)
	}
	return out, nil
}

type AgentHandler struct {
	Queries db.Querier
}

func NewAgentHandler(conn *pgxpool.Pool) *AgentHandler {
	return &AgentHandler{Queries: db.New(conn)}
}

type createAgentRequest struct {
	Name                  string      `json:"name"`
	Description           *string     `json:"description"`
	Type                  string      `json:"type"`
	IsActive              *bool       `json:"is_active"`
	WebhookUri            string      `json:"webhook_uri"`
	WebhookAllowedIps     []string    `json:"webhook_allowed_ips"`
	WebhookAllowedOrigins []string    `json:"webhook_allowed_origins"`
	MilvusCollection      string      `json:"milvus_collection"`
	WebhookInputField     string      `json:"webhook_input_field"`
	WebhookOutputField    string      `json:"webhook_output_field"`
	WebhookBodyFields     []BodyField `json:"webhook_body_fields"`
	WebhookHeaderFields   []BodyField `json:"webhook_header_fields"`
	Guardrail             string      `json:"guardrail"`
	Tags                  []string    `json:"tags"`
	Image                 *string     `json:"image"` // emoji string or object-storage URL
	CanAct                bool        `json:"can_act"`
	TemplateID            string      `json:"template_id"` // static template id ("product", "booking", ...) or empty
	// Whether the upstream serves webhook_uri + "/stream". Defaults true.
	WebhookStreamEnabled *bool `json:"webhook_stream_enabled"`
	// Webhook payload switches. persona gates tone/length/style,
	// guardrail gates the systemPrompt object. Both default true.
	PersonaEnabled   *bool `json:"persona_enabled"`
	GuardrailEnabled *bool `json:"guardrail_enabled"`
}

// buildMilvusCollection sanitizes the user-supplied base name and appends a
// random 8-char hex suffix so two agents named the same still get distinct
// Milvus collections. Milvus names allow only letters, digits and underscore,
// and must start with a letter or underscore.
func buildMilvusCollection(base string) string {
	base = strings.TrimSpace(base)
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == ' ':
			b.WriteByte('_')
		}
	}
	sanitized := strings.Trim(b.String(), "_")
	if sanitized == "" || (sanitized[0] >= '0' && sanitized[0] <= '9') {
		sanitized = "col_" + sanitized
	}
	return sanitized + "_" + lib.RandomHex(8)
}

// webhookFieldOrDefault falls back to the n8n defaults when the caller leaves
// the override blank.
func webhookFieldOrDefault(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: create api hit", "method", r.Method, "path", r.URL.Path)
	var req createAgentRequest

	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "agents: create invalid JSON body")
		return
	}
	slog.InfoContext(r.Context(), "agents: create request", "params", jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)
	req.MilvusCollection = strings.TrimSpace(req.MilvusCollection)

	if req.Name == "" {
		slog.WarnContext(r.Context(), "agents: create name is required", "status", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		slog.WarnContext(r.Context(), "agents: create webhook_uri is required", "status", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "webhook_uri are required")
		return
	}
	if req.MilvusCollection == "" {
		slog.WarnContext(r.Context(), "agents: create milvus_collection is required", "status", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "milvus_collection are required")
		return
	}

	agentType := db.AgentTypeCHAT
	switch req.Type {
	case "", string(db.AgentTypeCHAT):
		agentType = db.AgentTypeCHAT
	case string(db.AgentTypeREPORT):
		agentType = db.AgentTypeREPORT
	default:
		slog.WarnContext(r.Context(), "agents: create invalid type value", "status", http.StatusBadRequest, "type", req.Type)
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid type value")
		return
	}

	allowedIPs, err := parseAllowedIPs(req.WebhookAllowedIps)
	if err != nil {
		slog.WarnContext(r.Context(), "agents: create invalid allowed ips", "status", http.StatusBadRequest, "error", err)
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	agent, err := h.Queries.InsertAgent(r.Context(), db.InsertAgentParams{
		Name:                  req.Name,
		Description:           req.Description,
		Type:                  agentType,
		IsActive:              lib.NullBoolean(req.IsActive),
		WebhookUri:            req.WebhookUri,
		WebhookAllowedIps:     allowedIPs,
		WebhookAllowedOrigins: trimNonEmpty(req.WebhookAllowedOrigins),
		MilvusCollection:      buildMilvusCollection(req.MilvusCollection),
		WebhookInputField:     webhookFieldOrDefault(req.WebhookInputField, "chatInput"),
		WebhookOutputField:    webhookFieldOrDefault(req.WebhookOutputField, "output"),
		WebhookBodyFields:     marshalBodyFields(req.WebhookBodyFields),
		WebhookHeaderFields:   marshalBodyFields(req.WebhookHeaderFields),
		Guardrail:             strings.TrimSpace(req.Guardrail),
		Image:                 req.Image,
		CanAct:                req.CanAct,
		TemplateID:            strings.TrimSpace(req.TemplateID),
		// Omitted means "yes": today every upstream we ship serves /stream.
		WebhookStreamEnabled: req.WebhookStreamEnabled == nil || *req.WebhookStreamEnabled,
		PersonaEnabled:       req.PersonaEnabled == nil || *req.PersonaEnabled,
		GuardrailEnabled:     req.GuardrailEnabled == nil || *req.GuardrailEnabled,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: create db insert failed", "params", jsonForLog(req), "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	if err := h.syncAgentTags(r.Context(), agent.ID, req.Tags); err != nil {
		slog.ErrorContext(r.Context(), "agents: create tag sync failed", "agent_id", agent.ID, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to set agent tags")
		return
	}

	// Re-read so the response carries the freshly-linked tags.
	full, err := h.Queries.SelectAgentById(r.Context(), agent.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: create reload failed", "agent_id", agent.ID, "error", err)
		lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
		return
	}

	slog.InfoContext(r.Context(), "agents: create success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

func (h *AgentHandler) Read(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: read api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	isActive := lib.ParseParamsBool(params, "is_active")
	search := lib.ParseParamsString(params, "search")
	tagID := lib.ParseParamsUUID(params, "tag_id")
	slog.InfoContext(r.Context(), "agents: read request", "search", search, "is_active", isActive, "tag_id", tagID, "sort", pagination.Sort, "limit", pagination.Limit, "offset", pagination.Offset)

	agents, err := h.Queries.SelectAgents(r.Context(), db.SelectAgentsParams{
		Search:   search,
		IsActive: isActive,
		TagID:    tagID,
		Sort:     pagination.Sort,
		Limit:    pagination.Limit,
		Offset:   pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: read db select failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgents(r.Context(), db.CountAgentsParams{
		Search:   search,
		IsActive: isActive,
		TagID:    tagID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: read db count failed", "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	slog.InfoContext(r.Context(), "agents: read success", "status", http.StatusOK, "count", len(agents), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadByMcpId(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: read by mcp api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	mcp_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "agents: read by mcp invalid mcp id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	slog.InfoContext(r.Context(), "agents: read by mcp request", "mcp_id", mcp_id, "sort", pagination.Sort, "limit", pagination.Limit, "offset", pagination.Offset)

	agents, err := h.Queries.SelectAgentsByMcpId(r.Context(), db.SelectAgentsByMcpIdParams{
		McpID:  mcp_id,
		Sort:   pagination.Sort,
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: read by mcp db select failed", "mcp_id", mcp_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgentsByMcpId(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: read by mcp db count failed", "mcp_id", mcp_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	slog.InfoContext(r.Context(), "agents: read by mcp success", "status", http.StatusOK, "mcp_id", mcp_id, "count", len(agents), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadByKnowledgeId(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: read by knowledge api hit", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)
	knowledge_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "agents: read by knowledge invalid knowledge id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	slog.InfoContext(r.Context(), "agents: read by knowledge request", "knowledge_id", knowledge_id, "sort", pagination.Sort, "limit", pagination.Limit, "offset", pagination.Offset)

	agents, err := h.Queries.SelectAgentsByKnowledgeId(r.Context(), db.SelectAgentsByKnowledgeIdParams{
		KnowledgeID: knowledge_id,
		Sort:        pagination.Sort,
		Limit:       pagination.Limit,
		Offset:      pagination.Offset * pagination.Limit,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: read by knowledge db select failed", "knowledge_id", knowledge_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgentsByKnowledgeId(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: read by knowledge db count failed", "knowledge_id", knowledge_id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	slog.InfoContext(r.Context(), "agents: read by knowledge success", "status", http.StatusOK, "knowledge_id", knowledge_id, "count", len(agents), "total", totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: read by id api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "agents: read by id invalid id")
		return
	}
	slog.InfoContext(r.Context(), "agents: read by id request", "id", id)

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(r.Context(), "agents: read by id not found", "status", http.StatusNotFound, "agent_id", id)
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		slog.ErrorContext(r.Context(), "agents: read by id db select failed", "agent_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	slog.InfoContext(r.Context(), "agents: read by id success", "status", http.StatusOK, "response", jsonForLog(agent))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

type updateAgentRequest struct {
	Name                  string      `json:"name"`
	Description           *string     `json:"description"`
	IsActive              bool        `json:"is_active"`
	WebhookUri            string      `json:"webhook_uri"`
	WebhookAllowedIps     []string    `json:"webhook_allowed_ips"`
	WebhookAllowedOrigins []string    `json:"webhook_allowed_origins"`
	WebhookInputField     string      `json:"webhook_input_field"`
	WebhookOutputField    string      `json:"webhook_output_field"`
	WebhookBodyFields     []BodyField `json:"webhook_body_fields"`
	WebhookHeaderFields   []BodyField `json:"webhook_header_fields"`
	Guardrail             *string     `json:"guardrail"`
	Tags                  []string    `json:"tags"`
	Image                 *string     `json:"image"` // emoji string or object-storage URL
	WebhookStreamEnabled  *bool       `json:"webhook_stream_enabled"`
	// Omitted keeps the stored value (SQL COALESCE).
	PersonaEnabled   *bool `json:"persona_enabled"`
	GuardrailEnabled *bool `json:"guardrail_enabled"`
	// milvus_collection is intentionally omitted: it is immutable after create.
}

func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: update api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "agents: update invalid id")
		return
	}

	var req updateAgentRequest
	if !lib.ParseJSONBody(w, r, &req) {
		slog.ErrorContext(r.Context(), "agents: update invalid JSON body", "agent_id", id)
		return
	}
	slog.InfoContext(r.Context(), "agents: update request", "agent_id", id, "body", jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		slog.WarnContext(r.Context(), "agents: update name is required", "status", http.StatusBadRequest, "agent_id", id)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		slog.WarnContext(r.Context(), "agents: update webhook_uri is required", "status", http.StatusBadRequest, "agent_id", id)
		lib.ResponseJSONError(w, http.StatusBadRequest, "webhook_uri are required")
		return
	}

	allowedIPs, err := parseAllowedIPsOnUpdate(req.WebhookAllowedIps)
	if err != nil {
		slog.WarnContext(r.Context(), "agents: update invalid allowed ips", "status", http.StatusBadRequest, "agent_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	agent, err := h.Queries.UpdateAgent(r.Context(), db.UpdateAgentParams{
		Name:                  req.Name,
		Description:           req.Description,
		IsActive:              req.IsActive,
		WebhookUri:            req.WebhookUri,
		WebhookAllowedIps:     allowedIPs,
		WebhookAllowedOrigins: trimNonEmptyOnUpdate(req.WebhookAllowedOrigins),
		WebhookInputField:     webhookFieldOrDefault(req.WebhookInputField, "chatInput"),
		WebhookOutputField:    webhookFieldOrDefault(req.WebhookOutputField, "output"),
		WebhookBodyFields:     marshalBodyFieldsOnUpdate(req.WebhookBodyFields),
		WebhookHeaderFields:   marshalBodyFieldsOnUpdate(req.WebhookHeaderFields),
		Guardrail:             trimStringOnUpdate(req.Guardrail),
		Image:                 req.Image,
		WebhookStreamEnabled:  req.WebhookStreamEnabled,
		PersonaEnabled:        req.PersonaEnabled,
		GuardrailEnabled:      req.GuardrailEnabled,
		ID:                    id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(r.Context(), "agents: update agent not found", "status", http.StatusNotFound, "agent_id", id)
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		slog.ErrorContext(r.Context(), "agents: update db update failed", "agent_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	if err := h.syncAgentTags(r.Context(), agent.ID, req.Tags); err != nil {
		slog.ErrorContext(r.Context(), "agents: update tag sync failed", "agent_id", agent.ID, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to set agent tags")
		return
	}

	full, err := h.Queries.SelectAgentById(r.Context(), agent.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: update reload failed", "agent_id", agent.ID, "error", err)
		lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
		return
	}

	slog.InfoContext(r.Context(), "agents: update success", "status", http.StatusOK, "response", jsonForLog(full))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, full, nil)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "agents: delete api hit", "method", r.Method, "path", r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		slog.ErrorContext(r.Context(), "agents: delete invalid id")
		return
	}
	slog.InfoContext(r.Context(), "agents: delete request", "agent_id", id)

	rowsAffected, err := h.Queries.SoftDeleteAgent(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "agents: delete db soft-delete failed", "agent_id", id, "error", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	if rowsAffected == 0 {
		slog.WarnContext(r.Context(), "agents: delete agent not found", "status", http.StatusNotFound, "agent_id", id)
		lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	slog.InfoContext(r.Context(), "agents: delete success", "status", http.StatusNoContent, "rows_affected", rowsAffected)
	lib.ResponseJSONTemplate(w, http.StatusNoContent, nil, nil, nil)
}

type updateAgentPersonaRequest struct {
	Tone               string `json:"tone"`
	ResponseLength     string `json:"response_length"`
	CommunicationStyle string `json:"communication_style"`
}

func (h *AgentHandler) UpdatePersona(w http.ResponseWriter, r *http.Request) {
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		return
	}

	var req updateAgentPersonaRequest
	if !lib.ParseJSONBody(w, r, &req) {
		return
	}

	validTones := map[string]db.AgentTone{
		string(db.AgentToneFRIENDLY):     db.AgentToneFRIENDLY,
		string(db.AgentTonePROFESSIONAL): db.AgentTonePROFESSIONAL,
		string(db.AgentToneEXPLANATORY):  db.AgentToneEXPLANATORY,
	}
	validResponseLengths := map[string]db.AgentResponseLength{
		string(db.AgentResponseLengthSHORT):  db.AgentResponseLengthSHORT,
		string(db.AgentResponseLengthMEDIUM): db.AgentResponseLengthMEDIUM,
		string(db.AgentResponseLengthLONG):   db.AgentResponseLengthLONG,
	}
	validCommunicationStyles := map[string]db.AgentCommunicationStyle{
		string(db.AgentCommunicationStyleEXPERTADVISOR):       db.AgentCommunicationStyleEXPERTADVISOR,
		string(db.AgentCommunicationStyleEMPATHETICGUIDE):     db.AgentCommunicationStyleEMPATHETICGUIDE,
		string(db.AgentCommunicationStyleEFFICIENTCONCIERGE):  db.AgentCommunicationStyleEFFICIENTCONCIERGE,
		string(db.AgentCommunicationStyleEDUCATOR):            db.AgentCommunicationStyleEDUCATOR,
		string(db.AgentCommunicationStylePROACTIVECONSULTANT): db.AgentCommunicationStylePROACTIVECONSULTANT,
	}

	tone, ok := validTones[req.Tone]
	if !ok {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid tone value")
		return
	}

	responseLength, ok := validResponseLengths[req.ResponseLength]
	if !ok {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid response_length value")
		return
	}

	communicationStyle, ok := validCommunicationStyles[req.CommunicationStyle]
	if !ok {
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid communication_style value")
		return
	}

	agent, err := h.Queries.UpdateAgentPersona(r.Context(), db.UpdateAgentPersonaParams{
		Tone:               tone,
		ResponseLength:     responseLength,
		CommunicationStyle: communicationStyle,
		ID:                 id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update agent persona")
		return
	}

	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}
