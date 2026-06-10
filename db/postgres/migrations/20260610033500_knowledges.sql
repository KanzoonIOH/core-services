-- +goose Up
CREATE TABLE knowledges (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    description TEXT,
    source_type TEXT NOT NULL,
    source_uri TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE VIEW knowledges_view AS
SELECT
    id,
    name,
    description,
    source_type,
    source_uri,
    created_at,
    updated_at
FROM knowledges
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS knowledges_view;

DROP TABLE IF EXISTS knowledges;
