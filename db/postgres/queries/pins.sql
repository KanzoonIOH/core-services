-- name: InsertPin :one
-- Idempotent: re-pinning the same entity is a no-op that returns the existing
-- pin. Position is appended at the end of the user's current list.
INSERT INTO pins (user_id, entity_type, entity_id, position)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(entity_type),
    sqlc.arg(entity_id),
    COALESCE(
        (SELECT MAX(position) + 1 FROM pins WHERE user_id = sqlc.arg(user_id)),
        0
    )
)
ON CONFLICT (user_id, entity_type, entity_id)
DO UPDATE SET user_id = EXCLUDED.user_id
RETURNING id, user_id, entity_type, entity_id, position, created_at;

-- name: DeletePin :execrows
DELETE FROM pins
WHERE
    user_id = sqlc.arg(user_id)
    AND entity_type = sqlc.arg(entity_type)
    AND entity_id = sqlc.arg(entity_id);

-- name: DeletePinById :execrows
DELETE FROM pins
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: SelectPinnedEntityIds :many
-- Lightweight lookup for the frontend to know which entities are pinned (used to
-- toggle the pin button state) without resolving labels.
SELECT entity_type, entity_id
FROM pins
WHERE user_id = sqlc.arg(user_id);

-- name: SelectResolvedPins :many
-- The sidebar payload: each pin resolved to its current label + image via a
-- LEFT JOIN per entity type. Chat pins borrow the agent's name/image. Rows whose
-- target was deleted resolve to a NULL label and are dropped by the handler.
SELECT
    p.id,
    p.entity_type,
    p.entity_id,
    p.position,
    p.created_at,
    COALESCE(d.name, a.name, o.name, ca.name) AS label,
    COALESCE(a.image, o.image, ca.image) AS image,
    (
        d.id IS NOT NULL
        OR a.id IS NOT NULL
        OR o.id IS NOT NULL
        OR c.id IS NOT NULL
    )::bool AS resolved
FROM pins p
LEFT JOIN dashboards_view d
    ON p.entity_type = 'dashboard' AND d.id = p.entity_id
LEFT JOIN agents_view a
    ON p.entity_type = 'agent' AND a.id = p.entity_id
LEFT JOIN orchestrators_view o
    ON p.entity_type = 'orchestrator' AND o.id = p.entity_id
LEFT JOIN conversations_view c
    ON p.entity_type = 'chat' AND c.id = p.entity_id
LEFT JOIN agents_view ca
    ON ca.id = c.agent_id
WHERE p.user_id = sqlc.arg(user_id)
ORDER BY p.position ASC, p.created_at ASC;

-- name: UpdatePinPosition :execrows
UPDATE pins
SET position = sqlc.arg(position)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);
