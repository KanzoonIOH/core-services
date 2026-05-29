-- name: InsertKnowledge :one
INSERT INTO knowledges (name, description, source_type, source_uri)
VALUES (
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(source_type),
    sqlc.narg(source_uri)
)
RETURNING
    id, name, description, source_type, source_uri, created_at, updated_at;

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

-- name: SelectKnowledges :many
SELECT * FROM knowledges_view
WHERE (
    sqlc.narg('source_type')::text IS NULL
    OR sqlc.narg('source_type')::text = ''
    OR source_type = sqlc.narg('source_type')::text
)
ORDER BY
    CASE WHEN sqlc.narg('sort')::text = 'name_asc' THEN name END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'name_desc' THEN name END DESC,
    CASE WHEN sqlc.narg('sort')::text = 'created_asc' THEN created_at END ASC,
    CASE WHEN sqlc.narg('sort')::text = 'created_desc' THEN created_at END DESC,
    created_at DESC
LIMIT coalesce(sqlc.narg('limit'), 10) OFFSET coalesce(sqlc.narg('offset'), 0);

-- -- -- name: GetKnowledgeAgents :many
-- -- SELECT a.* FROM agents a
-- -- JOIN agent_knowledges ak ON ak.agent_id = a.id
-- -- WHERE
-- --     a.deleted_at IS NULL
-- --     AND ak.deleted_at IS NULL
-- --     AND ak.knowledge_id = sqlc.arg(knowledge_id)
-- -- ORDER BY a.created_at DESC;
