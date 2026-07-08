-- name: InsertKnowledge :one
INSERT INTO knowledges (name, description, source_type, source_uri, is_crawl)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(source_type),
    sqlc.narg(source_uri),
    sqlc.arg(is_crawl)
)
RETURNING
    id, name, description, source_type, source_uri, is_crawl, created_at, updated_at;

-- name: SelectKnowledgeById :one
SELECT * FROM knowledges_view
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateKnowledge :one
UPDATE knowledges
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING
    id, name, description, source_type, source_uri, created_at, updated_at;

-- name: SoftDeleteKnowledge :execrows
UPDATE knowledges
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: CountKnowledges :one
SELECT count(*) FROM knowledges_view
WHERE (
    sqlc.narg('source_type')::text IS NULL
    OR sqlc.narg('source_type')::text = ''
    OR source_type = sqlc.narg('source_type')::text
)
AND (
    sqlc.narg('search')::text IS NULL
    OR name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR description ILIKE '%' || sqlc.narg('search')::text || '%'
);

-- name: SelectKnowledges :many
SELECT
    kv.*,
    (
        SELECT count(*)
        FROM agent_knowledges_view akv
        WHERE akv.knowledge_id = kv.id
    ) AS agents_count
FROM knowledges_view kv
WHERE (
    sqlc.narg('source_type')::text IS NULL
    OR sqlc.narg('source_type')::text = ''
    OR kv.source_type = sqlc.narg('source_type')::text
)
AND (
    sqlc.narg('search')::text IS NULL
    OR kv.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR kv.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'source_type_asc' THEN source_type END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'source_type_desc' THEN source_type END DESC,
    created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountKnowledgesByAgentId :one
SELECT count(*)
FROM knowledges_view kv
JOIN agent_knowledges_view akv
    ON akv.knowledge_id = kv.id
WHERE akv.agent_id = sqlc.arg(agent_id)
AND (
    sqlc.narg('search')::text IS NULL
    OR kv.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR kv.description ILIKE '%' || sqlc.narg('search')::text || '%'
);

-- name: SelectKnowledgesByAgentId :many
SELECT
    kv.*,
    akv.status
FROM knowledges_view kv
JOIN agent_knowledges_view akv
    ON akv.knowledge_id = kv.id
WHERE akv.agent_id = sqlc.arg(agent_id)
AND (
    sqlc.narg('search')::text IS NULL
    OR kv.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR kv.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN kv.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN kv.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN kv.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN kv.created_at
    END DESC,
    kv.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAllKnowledges :one
SELECT count(*) FROM knowledges_view kv
WHERE (
    sqlc.narg('search')::text IS NULL
    OR kv.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR kv.description ILIKE '%' || sqlc.narg('search')::text || '%'
);

-- name: SelectKnowledgesWithAgentStatus :many
SELECT
    kv.*,
    (akv.id IS NOT NULL)::bool AS connected
FROM knowledges_view kv
LEFT JOIN agent_knowledges_view akv
    ON akv.knowledge_id = kv.id
    AND akv.agent_id = sqlc.arg(agent_id)
WHERE (
    sqlc.narg('search')::text IS NULL
    OR kv.name ILIKE '%' || sqlc.narg('search')::text || '%'
    OR kv.description ILIKE '%' || sqlc.narg('search')::text || '%'
)
ORDER BY
    connected DESC,
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN kv.name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN kv.name END DESC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN kv.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN kv.created_at
    END DESC,
    kv.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
