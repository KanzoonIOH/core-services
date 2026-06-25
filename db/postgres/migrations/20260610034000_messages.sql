-- +goose Up
CREATE TYPE MESSAGE_ROLE AS ENUM ('user', 'assistant', 'system');

-- No FK to conversations: the webhook proxy invents a conversation_id and never
-- inserts a conversations row. messages stands alone, keyed by conversation_id.
-- ponytail: add the FK when conversation rows are guaranteed to exist first.
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    conversation_id UUID NOT NULL,
    role MESSAGE_ROLE NOT NULL,
    -- Nullable: a report reply or attachments-only turn may carry no text.
    content TEXT,
    -- Uploaded docs/images for this message: [{url, name, mime, size}, ...].
    -- Stored by reference; the files themselves live in object storage (MinIO).
    attachments JSONB NOT NULL DEFAULT '[]',
    -- REPORT-agent structured reply (tables, chart specs, metrics). Free-form
    -- per vendor/report — JSONB so the shape evolves without migrations.
    data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Serves both "last N for context" and "full history, chronological".
CREATE INDEX idx_messages_conversation ON messages (conversation_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS messages;
DROP TYPE IF EXISTS MESSAGE_ROLE;
