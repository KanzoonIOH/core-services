-- name: SelectDropdownAgents :many
SELECT
    av.id,
    av.name,
    (akv.agent_id IS NOT NULL)::bool AS is_connected
FROM agents_view av
LEFT JOIN agent_knowledges_view akv
    ON
        akv.agent_id = av.id
        AND akv.knowledge_id = sqlc.arg(knowledge_id)
WHERE (
    sqlc.narg('search')::text IS NULL
    OR sqlc.narg('search')::text = ''
    OR av.name ILIKE '%' || sqlc.narg('search')::text || '%'
)
ORDER BY av.name ASC;
