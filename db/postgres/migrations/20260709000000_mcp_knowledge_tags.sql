-- +goose Up
-- Reuse the existing tags entity for MCPs and knowledges via their own junctions,
-- mirroring agent_tags. Renames/recolors on a tag propagate everywhere at once.
CREATE TABLE mcp_tags (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mcp_id UUID NOT NULL REFERENCES mcps (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mcp_tags_mcp_id ON mcp_tags (mcp_id);
CREATE INDEX idx_mcp_tags_tag_id ON mcp_tags (tag_id);
CREATE UNIQUE INDEX idx_mcp_tag_unique ON mcp_tags (mcp_id, tag_id);

CREATE TABLE knowledge_tags (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    knowledge_id UUID NOT NULL REFERENCES knowledges (id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_knowledge_tags_knowledge_id ON knowledge_tags (knowledge_id);
CREATE INDEX idx_knowledge_tags_tag_id ON knowledge_tags (tag_id);
CREATE UNIQUE INDEX idx_knowledge_tag_unique ON knowledge_tags (knowledge_id, tag_id);

-- +goose Down
DROP TABLE IF EXISTS knowledge_tags;
DROP TABLE IF EXISTS mcp_tags;
