-- +goose Up
-- is_crawl only meaningful for source_type='link': true = crawl the whole site,
-- false = index just this one page. Ignored for file-based knowledges.
ALTER TABLE knowledges
    ADD COLUMN is_crawl BOOLEAN NOT NULL DEFAULT false;

CREATE OR REPLACE VIEW knowledges_view AS
SELECT
    id,
    name,
    description,
    source_type,
    source_uri,
    created_at,
    updated_at,
    is_crawl
FROM knowledges
WHERE deleted_at IS NULL;

-- +goose Down
CREATE OR REPLACE VIEW knowledges_view AS
SELECT
    id,
    name,
    description,

ALTER TABLE knowledges
    DROP COLUMN is_crawl;
