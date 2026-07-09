-- name: InsertMcp :one
INSERT INTO mcps (name, description, uri, headers)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(uri),
    sqlc.arg(headers)
)
RETURNING id, name, description, uri, headers, created_at, updated_at;

-- name: SelectMcpById :one
SELECT
    m.*,
    (
        SELECT COALESCE(jsonb_agg(jsonb_build_object('id', t.id, 'name', t.name, 'color', t.color) ORDER BY t.name), '[]'::jsonb)
        FROM mcp_tags mt
        JOIN tags_view t ON t.id = mt.tag_id
        WHERE mt.mcp_id = m.id
    )::jsonb AS tags
FROM mcps_view m
WHERE m.id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateMcp :one
UPDATE mcps
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    uri = sqlc.arg(uri),
    headers = sqlc.arg(headers),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, name, description, uri, headers, created_at, updated_at;

-- name: SoftDeleteMcp :execrows
UPDATE mcps
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountMcps :one
SELECT count(*) FROM mcps_view m
WHERE (
    sqlc.narg('search')::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR m.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
AND (
    sqlc.narg('tag_id')::uuid IS NULL
    OR EXISTS (
        SELECT 1 FROM mcp_tags mt
        WHERE mt.mcp_id = m.id AND mt.tag_id = sqlc.narg('tag_id')::uuid
    )
);

-- name: SelectMcps :many
SELECT
    m.*,
    (
        SELECT count(*)
        FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) AS tools_count,
    (
        SELECT COALESCE(jsonb_agg(jsonb_build_object('id', t.id, 'name', t.name, 'color', t.color) ORDER BY t.name), '[]'::jsonb)
        FROM mcp_tags mt
        JOIN tags_view t ON t.id = mt.tag_id
        WHERE mt.mcp_id = m.id
    )::jsonb AS tags
FROM mcps_view AS m
WHERE (
    sqlc.narg('search')::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR m.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
AND (
    sqlc.narg('tag_id')::uuid IS NULL
    OR EXISTS (
        SELECT 1 FROM mcp_tags mt
        WHERE mt.mcp_id = m.id AND mt.tag_id = sqlc.narg('tag_id')::uuid
    )
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN m.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN m.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN m.created_at END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN m.created_at
    END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'tools_count_asc' THEN (
        SELECT count(*) FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'tools_count_desc' THEN (
        SELECT count(*) FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) END DESC,
    m.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountMcpsByAgentId :one
SELECT count(*)
FROM mcps_view m
JOIN agent_mcps_view amv
    ON amv.mcp_id = m.id
WHERE amv.agent_id = sqlc.arg(agent_id)
AND (
    sqlc.narg('search')::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR m.description ILIKE '%' || sqlc.narg('search')::text || '%'
);

-- name: SelectMcpsWithAgentStatus :many
SELECT
    m.*,
    (
        SELECT count(*)
        FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) AS tools_count,
    (amv.id IS NOT NULL)::bool AS connected
FROM mcps_view AS m
LEFT JOIN agent_mcps_view amv
    ON amv.mcp_id = m.id
    AND amv.agent_id = sqlc.arg(agent_id)
WHERE (
    sqlc.narg('search')::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR m.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
ORDER BY
    connected DESC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN m.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN m.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN m.created_at END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN m.created_at
    END DESC,
    m.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: SelectMcpsByAgentId :many
SELECT
    m.*,
    (
        SELECT count(*)
        FROM mcp_tools AS t
        WHERE t.mcp_id = m.id AND t.deleted_at IS NULL
    ) AS tools_count
FROM mcps_view AS m
JOIN agent_mcps_view amv
    ON amv.mcp_id = m.id
WHERE amv.agent_id = sqlc.arg(agent_id)
AND (
    sqlc.narg('search')::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR m.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN m.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN m.name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN m.created_at END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN m.created_at
    END DESC,
    m.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
