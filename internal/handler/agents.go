package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
	Name                  string   `json:"name"`
	Description           *string  `json:"description"`
	Type                  string   `json:"type"`
	IsActive              *bool    `json:"is_active"`
	WebhookUri            string   `json:"webhook_uri"`
	WebhookAllowedIps     []string `json:"webhook_allowed_ips"`
	WebhookAllowedOrigins []string `json:"webhook_allowed_origins"`
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-create] api hit method=%s path=%s", r.Method, r.URL.Path)
	var req createAgentRequest

	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][agent-create] api error invalid JSON body")
		return
	}
	log.Printf("[core-service][agent-create] request params=%s", jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		log.Printf("[core-service][agent-create] api error status=%d reason=name is required", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		log.Printf("[core-service][agent-create] api error status=%d reason=webhook_uri is required", http.StatusBadRequest)
		lib.ResponseJSONError(w, http.StatusBadRequest, "webhook_uri are required")
		return
	}

	agentType := db.AgentTypeCHAT
	switch req.Type {
	case "", string(db.AgentTypeCHAT):
		agentType = db.AgentTypeCHAT
	case string(db.AgentTypeREPORT):
		agentType = db.AgentTypeREPORT
	default:
		log.Printf("[core-service][agent-create] api error status=%d reason=invalid type value type=%q", http.StatusBadRequest, req.Type)
		lib.ResponseJSONError(w, http.StatusBadRequest, "invalid type value")
		return
	}

	allowedIPs, err := parseAllowedIPs(req.WebhookAllowedIps)
	if err != nil {
		log.Printf("[core-service][agent-create] api error status=%d reason=%v", http.StatusBadRequest, err)
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
	})
	if err != nil {
		log.Printf("[core-service][agent-create] db insert error params=%s error=%v", jsonForLog(req), err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	log.Printf("[core-service][agent-create] api success status=%d response=%s", http.StatusOK, jsonForLog(agent))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

func (h *AgentHandler) Read(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-read] api hit method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	isActive := lib.ParseParamsBool(params, "is_active")
	log.Printf("[core-service][agent-read] request params={is_active:%v sort:%v limit:%d offset:%d}", isActive, pagination.Sort, pagination.Limit, pagination.Offset)

	agents, err := h.Queries.SelectAgents(r.Context(), db.SelectAgentsParams{
		IsActive: isActive,
		Sort:     pagination.Sort,
		Limit:    pagination.Limit,
		Offset:   pagination.Offset * pagination.Limit,
	})
	if err != nil {
		log.Printf("[core-service][agent-read] db select error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgents(r.Context())
	if err != nil {
		log.Printf("[core-service][agent-read] db count error error=%v", err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	log.Printf("[core-service][agent-read] api success status=%d count=%d total=%d", http.StatusOK, len(agents), totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadByMcpId(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-read-by-mcp] api hit method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
	mcp_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][agent-read-by-mcp] api error invalid mcp id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	log.Printf("[core-service][agent-read-by-mcp] request params={mcp_id:%s sort:%v limit:%d offset:%d}", mcp_id, pagination.Sort, pagination.Limit, pagination.Offset)

	agents, err := h.Queries.SelectAgentsByMcpId(r.Context(), db.SelectAgentsByMcpIdParams{
		McpID:  mcp_id,
		Sort:   pagination.Sort,
		Limit:  pagination.Limit,
		Offset: pagination.Offset * pagination.Limit,
	})
	if err != nil {
		log.Printf("[core-service][agent-read-by-mcp] db select error mcp_id=%s error=%v", mcp_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgentsByMcpId(r.Context())
	if err != nil {
		log.Printf("[core-service][agent-read-by-mcp] db count error mcp_id=%s error=%v", mcp_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	log.Printf("[core-service][agent-read-by-mcp] api success status=%d mcp_id=%s count=%d total=%d", http.StatusOK, mcp_id, len(agents), totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadByKnowledgeId(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-read-by-knowledge] api hit method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
	knowledge_id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][agent-read-by-knowledge] api error invalid knowledge id")
		return
	}

	params := r.URL.Query()

	pagination := lib.ParsePaginationParams(params)
	log.Printf("[core-service][agent-read-by-knowledge] request params={knowledge_id:%s sort:%v limit:%d offset:%d}", knowledge_id, pagination.Sort, pagination.Limit, pagination.Offset)

	agents, err := h.Queries.SelectAgentsByKnowledgeId(r.Context(), db.SelectAgentsByKnowledgeIdParams{
		KnowledgeID: knowledge_id,
		Sort:        pagination.Sort,
		Limit:       pagination.Limit,
		Offset:      pagination.Offset * pagination.Limit,
	})
	if err != nil {
		log.Printf("[core-service][agent-read-by-knowledge] db select error knowledge_id=%s error=%v", knowledge_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	totalRow, err := h.Queries.CountAgentsByKnowledgeId(r.Context())
	if err != nil {
		log.Printf("[core-service][agent-read-by-knowledge] db count error knowledge_id=%s error=%v", knowledge_id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}

	log.Printf("[core-service][agent-read-by-knowledge] api success status=%d knowledge_id=%s count=%d total=%d", http.StatusOK, knowledge_id, len(agents), totalRow)
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agents, lib.ResponsePagination(int(pagination.Limit), int(pagination.Offset), len(agents), int(totalRow)))
}

func (h *AgentHandler) ReadById(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-read-by-id] api hit method=%s path=%s", r.Method, r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][agent-read-by-id] api error invalid id")
		return
	}
	log.Printf("[core-service][agent-read-by-id] request params={id:%s}", id)

	agent, err := h.Queries.SelectAgentById(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[core-service][agent-read-by-id] api error status=%d reason=agent not found id=%s", http.StatusNotFound, id)
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		log.Printf("[core-service][agent-read-by-id] db select error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	log.Printf("[core-service][agent-read-by-id] api success status=%d response=%s", http.StatusOK, jsonForLog(agent))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

type updateAgentRequest struct {
	Name                  string   `json:"name"`
	Description           *string  `json:"description"`
	IsActive              bool     `json:"is_active"`
	WebhookUri            string   `json:"webhook_uri"`
	WebhookAllowedIps     []string `json:"webhook_allowed_ips"`
	WebhookAllowedOrigins []string `json:"webhook_allowed_origins"`
}

func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-update] api hit method=%s path=%s", r.Method, r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][agent-update] api error invalid id")
		return
	}

	var req updateAgentRequest
	if !lib.ParseJSONBody(w, r, &req) {
		log.Printf("[core-service][agent-update] api error invalid JSON body id=%s", id)
		return
	}
	log.Printf("[core-service][agent-update] request params={id:%s body:%s}", id, jsonForLog(req))

	req.Name = strings.TrimSpace(req.Name)
	req.WebhookUri = strings.TrimSpace(req.WebhookUri)

	if req.Name == "" {
		log.Printf("[core-service][agent-update] api error status=%d reason=name is required id=%s", http.StatusBadRequest, id)
		lib.ResponseJSONError(w, http.StatusBadRequest, "name are required")
		return
	}
	if req.WebhookUri == "" {
		log.Printf("[core-service][agent-update] api error status=%d reason=webhook_uri is required id=%s", http.StatusBadRequest, id)
		lib.ResponseJSONError(w, http.StatusBadRequest, "webhook_uri are required")
		return
	}

	allowedIPs, err := parseAllowedIPs(req.WebhookAllowedIps)
	if err != nil {
		log.Printf("[core-service][agent-update] api error status=%d reason=%v id=%s", http.StatusBadRequest, err, id)
		lib.ResponseJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	agent, err := h.Queries.UpdateAgent(r.Context(), db.UpdateAgentParams{
		Name:                  req.Name,
		Description:           req.Description,
		IsActive:              req.IsActive,
		WebhookUri:            req.WebhookUri,
		WebhookAllowedIps:     allowedIPs,
		WebhookAllowedOrigins: trimNonEmpty(req.WebhookAllowedOrigins),
		ID:                    id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[core-service][agent-update] api error status=%d reason=agent not found id=%s", http.StatusNotFound, id)
			lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
			return
		}

		log.Printf("[core-service][agent-update] db update error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	log.Printf("[core-service][agent-update] api success status=%d response=%s", http.StatusOK, jsonForLog(agent))
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, agent, nil)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	log.Printf("[core-service][agent-delete] api hit method=%s path=%s", r.Method, r.URL.Path)
	id, ok := lib.ParseID(w, r, "id")
	if !ok {
		log.Printf("[core-service][agent-delete] api error invalid id")
		return
	}
	log.Printf("[core-service][agent-delete] request params={id:%s}", id)

	rowsAffected, err := h.Queries.SoftDeleteAgent(r.Context(), id)
	if err != nil {
		log.Printf("[core-service][agent-delete] db soft-delete error id=%s error=%v", id, err)
		lib.ResponseJSONError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	if rowsAffected == 0 {
		log.Printf("[core-service][agent-delete] api error status=%d reason=agent not found id=%s", http.StatusNotFound, id)
		lib.ResponseJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	log.Printf("[core-service][agent-delete] api success status=%d rows_affected=%d", http.StatusNoContent, rowsAffected)
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
