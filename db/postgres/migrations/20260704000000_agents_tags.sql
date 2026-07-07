-- +goose Up
-- Tags are a first-class entity so they can be renamed / recolored in one place
-- and have every agent reflect the change (via the agent_tags junction).
CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT '#64748b',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- Case-insensitive unique tag name among live tags (so "Auto" and "auto" don't
-- both exist). Upsert-by-name relies on this.
CREATE UNIQUE INDEX idx_tags_name_unique
ON tags (lower(name))
WHERE deleted_at IS null;

CREATE TABLE agent_tags (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_tags_agent_id ON agent_tags (agent_id);
CREATE INDEX idx_agent_tags_tag_id ON agent_tags (tag_id);
CREATE UNIQUE INDEX idx_agent_tag_unique ON agent_tags (agent_id, tag_id);

CREATE VIEW tags_view AS
SELECT id, name, color, created_at, updated_at
FROM tags
WHERE deleted_at IS null;

-- +goose Down
DROP VIEW IF EXISTS tags_view;
DROP TABLE IF EXISTS agent_tags;
DROP TABLE IF EXISTS tags;
