-- name: InsertMcpTool :one
INSERT INTO mcp_tools (mcp_id, name, description, target_uri, input_schema)
VALUES (
    sqlc.arg(mcp_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(target_uri),
    sqlc.narg(input_schema)
)
RETURNING
id, mcp_id, name, description, target_uri, input_schema, created_at, updated_at;

-- name: SelectMcpToolsByMcpId :many
SELECT * FROM mcp_tools_view
WHERE mcp_id = sqlc.arg(mcp_id)
ORDER BY name ASC;

-- name: CountMcpToolsByMcpId :one
SELECT count(*) FROM mcp_tools_view
WHERE mcp_id = sqlc.arg(mcp_id);

-- name: SoftDeleteMcpToolsByMcpId :execrows
UPDATE mcp_tools
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND mcp_id = sqlc.arg(mcp_id);
