-- name: SelectTags :many
SELECT
    t.id, t.name, t.color, t.created_at, t.updated_at,
    (
        SELECT COUNT(*)
        FROM agent_tags at
        JOIN agents a ON a.id = at.agent_id AND a.deleted_at IS NULL
        WHERE at.tag_id = t.id
    ) AS agents_count
FROM tags_view t
ORDER BY t.name;

-- name: UpsertTagByName :one
-- Insert a tag by name, or return the existing live one (case-insensitive).
-- ON CONFLICT targets the partial unique index on lower(name) WHERE deleted_at IS NULL.
INSERT INTO tags (name, color)
VALUES (sqlc.arg(name), sqlc.arg(color))
ON CONFLICT (lower(name)) WHERE deleted_at IS NULL
DO UPDATE SET name = tags.name
RETURNING id, name, color, created_at, updated_at;

-- name: UpdateTag :one
UPDATE tags
SET
    name = sqlc.arg(name),
    color = sqlc.arg(color),
    updated_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, name, color, created_at, updated_at;

-- name: SoftDeleteTag :execrows
UPDATE tags
SET deleted_at = NOW()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id);

-- name: DeleteAgentTags :exec
DELETE FROM agent_tags WHERE agent_id = sqlc.arg(agent_id);

-- name: InsertAgentTag :exec
INSERT INTO agent_tags (agent_id, tag_id)
VALUES (sqlc.arg(agent_id), sqlc.arg(tag_id))
ON CONFLICT (agent_id, tag_id) DO NOTHING;
