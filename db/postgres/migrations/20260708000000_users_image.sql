-- +goose Up
-- image holds either a URL (avatar picture in object storage) or an emoji string.
ALTER TABLE users ADD COLUMN image TEXT;

CREATE OR REPLACE VIEW users_view AS
SELECT
    id,
    name,
    username,
    email,
    role,
    created_at,
    updated_at,
    image
FROM users
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS users_view;
CREATE VIEW users_view AS
SELECT
    id,
    name,
    username,
    email,
    role,
    created_at,
    updated_at
FROM users
WHERE deleted_at IS NULL;

ALTER TABLE users DROP COLUMN IF EXISTS image;
