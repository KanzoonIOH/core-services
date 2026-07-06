-- +goose NO TRANSACTION
-- +goose Up
-- Invited users exist before they set a password, so hashed_password must be
-- nullable. Existing self-registered users always have one.
ALTER TABLE users ALTER COLUMN hashed_password DROP NOT NULL;

-- INVITE token type for the email-invite accept flow (set password -> VIEWER).
ALTER TYPE UPCOMING_CHANGES_TYPE ADD VALUE IF NOT EXISTS 'INVITE';

-- +goose Down
-- NOTE: postgres cannot drop an enum value; the INVITE value remains on down.
-- Backfill any null passwords before re-adding NOT NULL.
UPDATE users SET hashed_password = '' WHERE hashed_password IS NULL;
ALTER TABLE users ALTER COLUMN hashed_password SET NOT NULL;
