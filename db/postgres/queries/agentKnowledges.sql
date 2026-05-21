-- name: InsertAgentKnowledge :one
INSERT INTO agent_knowledges (agent_id, knowledge_id)
VALUES (
    sqlc.arg(agent_id),
    sqlc.arg(knowledge_id)
)
RETURNING *;

-- name: SelectAgentKnowledgesByAgentId :many
SELECT
    ak.*,
    sqlc.embed(k)
FROM agent_knowledges ak
JOIN knowledges k
    ON ak.knowledge_id = k.id
WHERE
    ak.deleted_at IS NULL
    AND ak.agent_id = sqlc.arg(agent_id)
LIMIT
    coalesce(sqlc.narg('limit'), 10)
    OFFSET coalesce(sqlc.narg('offset'), 0);

-- name: UpdateAgentKnowledge :one
UPDATE agent_knowledges
SET
    is_active_prod = sqlc.arg(is_active_prod),
    is_active_dev = sqlc.arg(is_active_dev),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING *;

-- name: SoftDeleteAgentKnowledge :execrows
UPDATE agent_knowledges
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);
