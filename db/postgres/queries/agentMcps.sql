-- name: InsertAgentMcp :one
INSERT INTO agent_mcps (agent_id, mcp_id)
VALUES (
    sqlc.arg(agent_id),
    sqlc.arg(mcp_id)
)
RETURNING
    id, agent_id, mcp_id, created_at, updated_at;

-- name: SoftDeleteAgentMcp :execrows
UPDATE agent_mcps
SET deleted_at = NOW()
WHERE
    deleted_at IS NULL
    AND agent_id = sqlc.arg(agent_id)
    AND mcp_id = sqlc.arg(mcp_id);
