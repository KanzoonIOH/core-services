-- +goose Up
CREATE TYPE CONVERSATION_END_REASON AS ENUM (
    'ESCALATED',
    'TIMED_OUT',
    'HUMAN_CONFIRMED',
    'HUMAN_INTERCEPTED'
);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    is_active BOOLEAN NOT NULL DEFAULT true,
    agent_id UUID NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    -- end_reason: the mechanical trigger that closed the conversation.
    end_reason CONVERSATION_END_REASON,
    -- is_resolved: the quality outcome, set later by the enrichment worker.
    -- NULL until analysed; can be TRUE even for TIMED_OUT when the
    -- conversation was not left hanging (AI not awaiting a user reply).
    is_resolved BOOLEAN,
    deleted_at TIMESTAMPTZ,
    message_count INTEGER NOT NULL DEFAULT 0,
    resolution_ms BIGINT
);

CREATE INDEX idx_conversations_agent_id ON conversations (agent_id)
WHERE deleted_at IS NULL;

CREATE INDEX idx_conversations_is_active ON conversations (is_active)
WHERE deleted_at IS NULL;

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
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS conversations_view;

DROP TABLE IF EXISTS conversations;

DROP TYPE IF EXISTS CONVERSATION_END_REASON;
