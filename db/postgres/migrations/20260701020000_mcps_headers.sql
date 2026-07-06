-- +goose Up
-- Custom HTTP headers sent when connecting to the MCP server (e.g.
-- Authorization: Bearer ...). Stored as a flat string->string map.
-- ponytail: plaintext secrets, readable by any authed admin via mcps_view.
-- Encrypt / mask on read if MCP creds ever cross a trust boundary.
ALTER TABLE mcps ADD COLUMN headers JSONB NOT NULL DEFAULT '{}';

CREATE OR REPLACE VIEW mcps_view AS
SELECT
    id,
    name,
    description,
    uri,
    created_at,
    updated_at,
    headers
FROM mcps
WHERE deleted_at IS NULL;

-- +goose Down
-- CREATE OR REPLACE VIEW cannot drop a column, so drop and recreate.
DROP VIEW IF EXISTS mcps_view;
CREATE VIEW mcps_view AS
SELECT
    id,
    name,
    description,
    uri,
    created_at,
    updated_at
FROM mcps
WHERE deleted_at IS NULL;

ALTER TABLE mcps DROP COLUMN IF EXISTS headers;
