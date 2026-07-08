-- name: GetGlobalConfig :one
SELECT id, agent_name, industry_description, guardrail, created_at, updated_at
FROM global_config
WHERE id = true;

-- name: UpdateGlobalConfig :one
UPDATE global_config
SET
    agent_name = sqlc.arg(agent_name),
    industry_description = sqlc.arg(industry_description),
    guardrail = sqlc.arg(guardrail),
    updated_at = now()
WHERE id = true
RETURNING id, agent_name, industry_description, guardrail, created_at, updated_at;
