-- name: InsertOrchestrator :one
INSERT INTO orchestrators (name, description, is_active, orchestrator_agent_id, routing_guide, persona, guardrail, webhook_uri, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, webhook_stream_enabled, persona_enabled, guardrail_enabled)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(is_active),
    sqlc.arg(orchestrator_agent_id),
    sqlc.arg(routing_guide),
    sqlc.arg(persona),
    sqlc.arg(guardrail),
    sqlc.arg(webhook_uri),
    sqlc.arg(webhook_input_field),
    sqlc.arg(webhook_output_field),
    sqlc.arg(webhook_body_fields),
    sqlc.arg(webhook_header_fields),
    sqlc.arg(webhook_stream_enabled),
    sqlc.arg(persona_enabled),
    sqlc.arg(guardrail_enabled)
)
RETURNING
    id, name, description, is_active, orchestrator_agent_id, routing_guide, persona, guardrail, image, webhook_uri, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, webhook_stream_enabled, persona_enabled, guardrail_enabled, created_at, updated_at;

-- name: InsertOrchestratorAgent :one
INSERT INTO orchestrator_agents (orchestrator_id, agent_id, tool_name, description)
VALUES (
    sqlc.arg(orchestrator_id),
    sqlc.arg(agent_id),
    sqlc.arg(tool_name),
    sqlc.arg(description)
)
RETURNING
    id, orchestrator_id, agent_id, tool_name, description, created_at, updated_at;

-- name: UpdateOrchestrator :one
UPDATE orchestrators
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    is_active = sqlc.arg(is_active),
    -- Omitted (NULL) keeps the stored text; an empty string clears it. These
    -- are generated upstream at create time and are expensive to lose.
    routing_guide = COALESCE(sqlc.narg(routing_guide), routing_guide),
    persona = COALESCE(sqlc.narg(persona), persona),
    guardrail = COALESCE(sqlc.narg(guardrail), guardrail),
    image = sqlc.narg(image),
    webhook_uri = sqlc.arg(webhook_uri),
    webhook_input_field = sqlc.arg(webhook_input_field),
    webhook_output_field = sqlc.arg(webhook_output_field),
    -- NULL means "field omitted by the caller": keep what is stored. An empty
    -- JSON array clears it. Stops partial clients from wiping stored auth.
    webhook_body_fields = COALESCE(sqlc.narg(webhook_body_fields), webhook_body_fields),
    webhook_header_fields = COALESCE(sqlc.narg(webhook_header_fields), webhook_header_fields),
    webhook_stream_enabled = COALESCE(sqlc.narg(webhook_stream_enabled), webhook_stream_enabled),
    persona_enabled = COALESCE(sqlc.narg(persona_enabled), persona_enabled),
    guardrail_enabled = COALESCE(sqlc.narg(guardrail_enabled), guardrail_enabled),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING
    id, name, description, is_active, orchestrator_agent_id, routing_guide, persona, guardrail, image, webhook_uri, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, webhook_stream_enabled, persona_enabled, guardrail_enabled, created_at, updated_at;

-- name: SoftDeleteOrchestrator :execrows
UPDATE orchestrators
SET deleted_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountOrchestrators :one
SELECT COUNT(*) FROM orchestrators_view ov
WHERE (
    sqlc.narg('search')::text IS NULL
    OR ov.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR ov.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
AND (
    sqlc.narg('is_active')::bool IS NULL
    OR ov.is_active = sqlc.narg('is_active')::bool
);

-- name: SelectOrchestrators :many
SELECT
    ov.*,
    COALESCE(oa.agents_count, 0) AS agents_count
FROM orchestrators_view ov
LEFT JOIN (
    SELECT
        orchestrator_id,
        COUNT(*) AS agents_count
    FROM orchestrator_agents_view
    GROUP BY orchestrator_id
) oa ON oa.orchestrator_id = ov.id
WHERE (
    sqlc.narg('search')::text IS NULL
    OR ov.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR ov.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
AND (
    sqlc.narg('is_active')::bool IS NULL
    OR ov.is_active = sqlc.narg('is_active')::bool
)
ORDER BY
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_asc' THEN ov.is_active
    END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_desc' THEN ov.is_active
    END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN ov.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN ov.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN ov.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN ov.created_at
    END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'modified_asc' THEN ov.updated_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'modified_desc' THEN ov.updated_at
    END DESC,
    ov.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: SelectOrchestratorById :one
SELECT ov.* FROM orchestrators_view ov
WHERE ov.id = sqlc.arg(id)
LIMIT 1;

-- name: SelectOrchestratorAgents :many
-- Per-agent mapping joined to the agent row so callers get the derived fields
-- (endpoint == agent.template_id, can_act) without us storing them redundantly.
SELECT
    oav.id,
    oav.orchestrator_id,
    oav.agent_id,
    oav.tool_name,
    oav.description,
    av.name AS agent_name,
    av.template_id AS endpoint,
    av.can_act,
    av.image AS agent_image
FROM orchestrator_agents_view oav
JOIN agents_view av ON av.id = oav.agent_id
WHERE oav.orchestrator_id = sqlc.arg(orchestrator_id)
ORDER BY oav.created_at ASC;
