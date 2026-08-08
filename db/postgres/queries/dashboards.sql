-- name: InsertDashboard :one
INSERT INTO dashboards (owner_id, name, description, config)
VALUES (
    sqlc.arg(owner_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(config)
)
RETURNING id, owner_id, name, description, config, visibility, published_at, published_by, created_at, updated_at;

-- name: SelectDashboardById :one
SELECT * FROM dashboards_view
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateDashboard :one
-- Owner-scoped: the WHERE clause enforces that only the owner's row is touched.
UPDATE dashboards
SET
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    config = sqlc.arg(config),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
    AND owner_id = sqlc.arg(owner_id)
RETURNING id, owner_id, name, description, config, visibility, published_at, published_by, created_at, updated_at;

-- name: SoftDeleteDashboard :execrows
-- Owner-scoped delete.
UPDATE dashboards
SET deleted_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
    AND owner_id = sqlc.arg(owner_id);

-- name: PublishDashboard :one
-- Admin-gated at the handler; records who published and when.
UPDATE dashboards
SET
    visibility = 'published',
    published_at = now(),
    published_by = sqlc.arg(published_by),
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, owner_id, name, description, config, visibility, published_at, published_by, created_at, updated_at;

-- name: UnpublishDashboard :one
UPDATE dashboards
SET
    visibility = 'private',
    published_at = NULL,
    published_by = NULL,
    updated_at = now()
WHERE
    deleted_at IS NULL
    AND id = sqlc.arg(id)
RETURNING id, owner_id, name, description, config, visibility, published_at, published_by, created_at, updated_at;

-- name: SelectMyDashboards :many
-- Everything the caller can see in their sidebar: dashboards they own PLUS
-- published dashboards they've imported. `is_owner` distinguishes edit rights;
-- `imported` flags the ones that came from someone else.
SELECT
    d.*,
    (d.owner_id = sqlc.arg(user_id))::bool AS is_owner,
    (di.id IS NOT NULL)::bool AS imported,
    u.name AS owner_name
FROM dashboards_view d
LEFT JOIN dashboard_imports di
    ON di.dashboard_id = d.id AND di.user_id = sqlc.arg(user_id)
JOIN users u ON u.id = d.owner_id
WHERE
    d.owner_id = sqlc.arg(user_id)
    OR di.id IS NOT NULL
ORDER BY is_owner DESC, d.created_at DESC;

-- name: SelectPublishedDashboards :many
-- The team catalog: all published dashboards, with a flag telling the caller
-- which ones they've already imported and whether they own it.
SELECT
    d.*,
    (d.owner_id = sqlc.arg(user_id))::bool AS is_owner,
    (di.id IS NOT NULL)::bool AS imported,
    u.name AS owner_name
FROM dashboards_view d
LEFT JOIN dashboard_imports di
    ON di.dashboard_id = d.id AND di.user_id = sqlc.arg(user_id)
JOIN users u ON u.id = d.owner_id
WHERE d.visibility = 'published'
ORDER BY d.published_at DESC;

-- name: InsertDashboardImport :execrows
-- Idempotent import: ON CONFLICT means re-importing is a no-op.
INSERT INTO dashboard_imports (dashboard_id, user_id)
VALUES (sqlc.arg(dashboard_id), sqlc.arg(user_id))
ON CONFLICT (dashboard_id, user_id) DO NOTHING;

-- name: DeleteDashboardImport :execrows
DELETE FROM dashboard_imports
WHERE dashboard_id = sqlc.arg(dashboard_id) AND user_id = sqlc.arg(user_id);

-- name: SelectDashboardImport :one
SELECT * FROM dashboard_imports
WHERE dashboard_id = sqlc.arg(dashboard_id) AND user_id = sqlc.arg(user_id)
LIMIT 1;
