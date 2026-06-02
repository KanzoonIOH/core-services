-- name: InsertAgentKnowledge :one
INSERT INTO agent_knowledges (agent_id, knowledge_id)
VALUES (
    sqlc.arg(agent_id),
    sqlc.arg(knowledge_id)
)
RETURNING
    id, agent_id, knowledge_id, is_active_prod, is_active_dev, created_at, updated_at;

-- name: CountAgentKnowledgesByAgentId :one
SELECT COUNT(*)
FROM agent_knowledges_view akv
JOIN knowledges_view kv
    ON akv.knowledge_id = kv.id
WHERE akv.agent_id = sqlc.arg(agent_id);

-- name: SelectAgentKnowledgesByAgentId :many
SELECT
    akv.*,
    sqlc.embed(kv)
FROM agent_knowledges_view akv
JOIN knowledges_view kv
    ON akv.knowledge_id = kv.id
WHERE akv.agent_id = sqlc.arg(agent_id)
ORDER BY
    CASE
        WHEN sqlc.narg('sort')::text = 'created_asc' THEN akv.created_at
    END ASC,
    CASE
        WHEN sqlc.narg('sort')::text = 'created_desc' THEN akv.created_at
    END DESC,
    akv.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateAgentKnowledge :one
UPDATE agent_knowledges
SET
    is_active_prod = sqlc.arg(is_active_prod),
    is_active_dev = sqlc.arg(is_active_dev),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING
    id, agent_id, knowledge_id, is_active_prod, is_active_dev, created_at, updated_at;

-- name: SoftDeleteAgentKnowledge :execrows
UPDATE agent_knowledges
SET deleted_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);
