-- +goose Up
CREATE TYPE AGENT_TYPE AS ENUM (
    'CHAT',
    'REPORT'
);

CREATE TYPE AGENT_TONE AS ENUM (
    'FRIENDLY',
    'PROFESSIONAL',
    'EXPLANATORY'
);

CREATE TYPE AGENT_RESPONSE_LENGTH AS ENUM (
    'SHORT',
    'MEDIUM',
    'LONG'
);

CREATE TYPE AGENT_COMMUNICATION_STYLE AS ENUM (
    'EXPERT_ADVISOR',
    'EMPATHETIC_GUIDE',
    'EFFICIENT_CONCIERGE',
    'EDUCATOR',
    'PROACTIVE_CONSULTANT'
);

CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    description TEXT,
    type AGENT_TYPE NOT NULL DEFAULT 'CHAT',
    is_active BOOLEAN NOT NULL DEFAULT false,
    webhook_uri TEXT NOT NULL,
    -- Empty array means allow all callers / browser origins.
    webhook_allowed_ips INET[] NOT NULL DEFAULT '{}',
    webhook_allowed_origins TEXT[] NOT NULL DEFAULT '{}',
    tone AGENT_TONE NOT NULL DEFAULT 'FRIENDLY',
    response_length AGENT_RESPONSE_LENGTH NOT NULL DEFAULT 'MEDIUM',
    communication_style AGENT_COMMUNICATION_STYLE NOT NULL
    DEFAULT 'EXPERT_ADVISOR',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    type,
    is_active,
    webhook_uri,
    webhook_allowed_ips,
    webhook_allowed_origins,
    tone,
    response_length,
    communication_style,
    created_at,
    updated_at
FROM agents
WHERE deleted_at IS null;

-- +goose Down
DROP VIEW IF EXISTS agents_view;

DROP TABLE IF EXISTS agents;

DROP TYPE IF EXISTS AGENT_COMMUNICATION_STYLE;
DROP TYPE IF EXISTS AGENT_RESPONSE_LENGTH;
DROP TYPE IF EXISTS AGENT_TONE;
DROP TYPE IF EXISTS AGENT_TYPE;
