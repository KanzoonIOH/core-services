-- name: InsertAgent :one
INSERT INTO agents (name, description, type, is_active, webhook_uri, webhook_allowed_ips, webhook_allowed_origins, milvus_collection, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, guardrail, image, can_act, template_id, webhook_stream_enabled, persona_enabled, guardrail_enabled)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(type),
    sqlc.arg(is_active),
    sqlc.arg(webhook_uri),
    sqlc.arg(webhook_allowed_ips),
    sqlc.arg(webhook_allowed_origins),
    sqlc.arg(milvus_collection),
    sqlc.arg(webhook_input_field),
    sqlc.arg(webhook_output_field),
    sqlc.arg(webhook_body_fields),
    sqlc.arg(webhook_header_fields),
    sqlc.arg(guardrail),
    sqlc.narg(image),
    sqlc.arg(can_act),
    sqlc.arg(template_id),
    sqlc.arg(webhook_stream_enabled),
    sqlc.arg(persona_enabled),
    sqlc.arg(guardrail_enabled)
)
RETURNING
    id, name, description, type, is_active, webhook_uri, webhook_allowed_ips, webhook_allowed_origins, tone, response_length, communication_style, created_at, updated_at, milvus_collection, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, guardrail, image, can_act, template_id, webhook_stream_enabled, persona_enabled, guardrail_enabled;

-- name: SelectAgentById :one
SELECT
    av.*,
    (
        SELECT COUNT(*) FROM agent_knowledges_view akv
        WHERE akv.agent_id = av.id
    ) AS knowledges_count,
    (
        SELECT COUNT(*) FROM agent_mcps_view amv
        WHERE amv.agent_id = av.id
    ) AS mcps_count,
    (
        SELECT COALESCE(jsonb_agg(jsonb_build_object('id', t.id, 'name', t.name, 'color', t.color) ORDER BY t.name), '[]'::jsonb)
        FROM agent_tags at
        JOIN tags_view t ON t.id = at.tag_id
        WHERE at.agent_id = av.id
    )::jsonb AS tags
FROM agents_view av
WHERE av.id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateAgent :one
UPDATE agents
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    is_active = sqlc.arg(is_active),
    webhook_uri = sqlc.arg(webhook_uri),
    -- NULL means "field omitted by the caller": keep what is stored. An empty
    -- array clears it. Stops partial clients (e.g. the Persona tab, which only
    -- means to change the guardrail) from wiping stored auth and allowlists.
    webhook_allowed_ips = COALESCE(sqlc.narg(webhook_allowed_ips), webhook_allowed_ips),
    webhook_allowed_origins = COALESCE(sqlc.narg(webhook_allowed_origins), webhook_allowed_origins),
    webhook_input_field = sqlc.arg(webhook_input_field),
    webhook_output_field = sqlc.arg(webhook_output_field),
    webhook_body_fields = COALESCE(sqlc.narg(webhook_body_fields), webhook_body_fields),
    webhook_header_fields = COALESCE(sqlc.narg(webhook_header_fields), webhook_header_fields),
    guardrail = COALESCE(sqlc.narg(guardrail), guardrail),
    image = sqlc.narg(image),
    webhook_stream_enabled = COALESCE(sqlc.narg(webhook_stream_enabled), webhook_stream_enabled),
    persona_enabled = COALESCE(sqlc.narg(persona_enabled), persona_enabled),
    guardrail_enabled = COALESCE(sqlc.narg(guardrail_enabled), guardrail_enabled),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING
    id, name, description, type, is_active, webhook_uri, webhook_allowed_ips, webhook_allowed_origins, tone, response_length, communication_style, created_at, updated_at, milvus_collection, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, guardrail, image, can_act, template_id, webhook_stream_enabled, persona_enabled, guardrail_enabled;

-- name: UpdateAgentPersona :one
UPDATE agents
SET
    tone = sqlc.arg(tone),
    response_length = sqlc.arg(response_length),
    communication_style = sqlc.arg(communication_style),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING
    id, name, description, type, is_active, webhook_uri, webhook_allowed_ips, webhook_allowed_origins, tone, response_length, communication_style, created_at, updated_at, milvus_collection, webhook_input_field, webhook_output_field, webhook_body_fields, webhook_header_fields, guardrail, image, can_act, template_id, webhook_stream_enabled, persona_enabled, guardrail_enabled;

-- name: SoftDeleteAgent :execrows
UPDATE agents
SET deleted_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountAgents :one
SELECT COUNT(*) FROM agents_view av
WHERE (
    sqlc.narg('search')::text IS NULL
    OR av.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR av.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
AND (
    sqlc.narg('is_active')::bool IS NULL
    OR av.is_active = sqlc.narg('is_active')::bool
)
AND (
    sqlc.narg('tag_id')::uuid IS NULL
    OR EXISTS (
        SELECT 1 FROM agent_tags at
        WHERE at.agent_id = av.id AND at.tag_id = sqlc.narg('tag_id')::uuid
    )
);

-- name: SelectAgents :many
SELECT
    av.*,
    COALESCE(ak.knowledges_count, 0) AS knowledges_count,
    COALESCE(m.mcps_count, 0) AS mcps_count,
    (
        SELECT COALESCE(jsonb_agg(jsonb_build_object('id', t.id, 'name', t.name, 'color', t.color) ORDER BY t.name), '[]'::jsonb)
        FROM agent_tags at
        JOIN tags_view t ON t.id = at.tag_id
        WHERE at.agent_id = av.id
    )::jsonb AS tags
FROM agents_view av
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*) AS knowledges_count
    FROM agent_knowledges_view
    GROUP BY agent_id
) ak ON ak.agent_id = av.id
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*) AS mcps_count
    FROM agent_mcps_view
    GROUP BY agent_id
) m ON m.agent_id = av.id
WHERE (
    sqlc.narg('search')::text IS NULL
    OR av.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR av.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
AND (
    sqlc.narg('is_active')::bool IS NULL
    OR av.is_active = sqlc.narg('is_active')::bool
)
AND (
    sqlc.narg('tag_id')::uuid IS NULL
    OR EXISTS (
        SELECT 1 FROM agent_tags at
        WHERE at.agent_id = av.id AND at.tag_id = sqlc.narg('tag_id')::uuid
    )
)
ORDER BY
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_asc' THEN av.is_active
    END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'is_active_desc' THEN av.is_active
    END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN av.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN av.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN av.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN av.created_at
    END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'modified_asc' THEN av.updated_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'modified_desc' THEN av.updated_at
    END DESC,
    av.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAgentsByMcpId :one
SELECT COUNT(*) FROM agents_view;

-- name: SelectAgentsByMcpId :many
SELECT
    av.*,
    (amv.id IS NOT NULL)::bool AS connected
FROM agents_view av
LEFT JOIN agent_mcps_view amv
    ON amv.agent_id = av.id
    AND amv.mcp_id = sqlc.arg(mcp_id)
ORDER BY
    connected DESC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN av.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN av.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN av.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN av.created_at
    END DESC,
    av.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAgentsByKnowledgeId :one
SELECT COUNT(*) FROM agents_view;

-- name: SelectAgentsByKnowledgeId :many
SELECT
    av.*,
    (akv.id IS NOT NULL)::bool AS connected
FROM agents_view av
LEFT JOIN agent_knowledges_view akv
    ON akv.agent_id = av.id
    AND akv.knowledge_id = sqlc.arg(knowledge_id)
ORDER BY
    connected DESC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN av.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN av.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN av.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN av.created_at
    END DESC,
    av.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
