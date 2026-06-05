-- +goose Up
ALTER TABLE mcp_tools ADD COLUMN input_schema JSONB;

CREATE VIEW mcp_tools_view AS
SELECT
    id,
    mcp_id,
    name,
    description,
    target_uri,
    input_schema,
    created_at,
    updated_at
FROM mcp_tools
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS mcp_tools_view;
ALTER TABLE mcp_tools DROP COLUMN input_schema;
