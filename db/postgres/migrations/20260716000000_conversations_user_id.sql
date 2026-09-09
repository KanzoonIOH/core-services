-- +goose Up
-- Owner of a conversation, so the viewer app can list only its own chats.
-- Nullable: API-key chat traffic has no user behind it, and pre-existing rows
-- have no owner either.
ALTER TABLE conversations
ADD COLUMN user_id UUID REFERENCES users (id) ON DELETE SET NULL;

CREATE INDEX idx_conversations_user_id ON conversations (user_id)
WHERE deleted_at IS null;

-- Appended at the end: CREATE OR REPLACE VIEW cannot reorder existing columns.
CREATE OR REPLACE VIEW conversations_view AS
SELECT
    id,
    is_active,
    agent_id,
    started_at,
    ended_at,
    end_reason,
    is_resolved,
    message_count,
    resolution_ms,
    user_id
FROM conversations
WHERE deleted_at IS null;

-- +goose Down
DROP VIEW conversations_view;

DROP INDEX IF EXISTS idx_conversations_user_id;

ALTER TABLE conversations
DROP COLUMN user_id;

CREATE VIEW conversations_view AS
SELECT
    id,
    is_active,
    agent_id,
    started_at,
    ended_at,
    end_reason,
    is_resolved,
    message_count,
    resolution_ms
FROM conversations
WHERE deleted_at IS null;
