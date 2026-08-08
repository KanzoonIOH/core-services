-- +goose Up
-- Customizable dashboards. A dashboard is owned by exactly one user (owner_id)
-- and only that owner may edit or delete it. The `config` JSONB holds the widget
-- list: each widget names a data-source template (an existing /logs/* endpoint)
-- plus user-tunable params (range, agent_id, metric, ...) — granular enough for
-- marketing users without exposing raw queries.
--
-- Visibility: 'private' (only owner sees it) or 'published' (any team member can
-- discover + import it). Publishing is admin-gated at the handler layer.
--
-- Imports are REFERENCES, not copies (see dashboard_imports): an imported
-- dashboard always renders from the owner's live `config`, so owner edits sync
-- to every importer instantly and importers are inherently read-only.
CREATE TABLE dashboards (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    owner_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    config JSONB NOT NULL DEFAULT '{"widgets": []}'::jsonb,
    visibility TEXT NOT NULL DEFAULT 'private',
    published_at TIMESTAMPTZ,
    published_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT dashboards_visibility_check
    CHECK (visibility IN ('private', 'published'))
);

CREATE INDEX idx_dashboards_owner_id ON dashboards (owner_id)
WHERE deleted_at IS NULL;

CREATE INDEX idx_dashboards_visibility ON dashboards (visibility)
WHERE deleted_at IS NULL;

-- A user's subscription to a published dashboard. One row per (dashboard, user).
-- Deleting either side cascades. The owner never needs an import row — they see
-- their own dashboards directly.
CREATE TABLE dashboard_imports (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    dashboard_id UUID NOT NULL
    REFERENCES dashboards (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_dashboard_import_unique
ON dashboard_imports (dashboard_id, user_id);

CREATE INDEX idx_dashboard_imports_user_id
ON dashboard_imports (user_id);

CREATE VIEW dashboards_view AS
SELECT
    id,
    owner_id,
    name,
    description,
    config,
    visibility,
    published_at,
    published_by,
    created_at,
    updated_at
FROM dashboards
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS dashboards_view;

DROP TABLE IF EXISTS dashboard_imports;
DROP TABLE IF EXISTS dashboards;
