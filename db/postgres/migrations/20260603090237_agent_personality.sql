-- +goose Up
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

ALTER TABLE agents
    ADD COLUMN tone AGENT_TONE NOT NULL DEFAULT 'FRIENDLY',
    ADD COLUMN response_length AGENT_RESPONSE_LENGTH NOT NULL DEFAULT 'MEDIUM',
    ADD COLUMN communication_style AGENT_COMMUNICATION_STYLE NOT NULL DEFAULT 'EXPERT_ADVISOR';

DROP VIEW agents_view;

CREATE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    is_active,
    webhook_uri,
    tone,
    response_length,
    communication_style,
    created_at,
    updated_at
FROM agents
WHERE deleted_at IS NULL;

-- +goose Down
DROP VIEW agents_view;

ALTER TABLE agents
    DROP COLUMN tone,
    DROP COLUMN response_length,
    DROP COLUMN communication_style;

CREATE VIEW agents_view AS
SELECT
    id,
    name,
    description,
    is_active,
    webhook_uri,
    created_at,
    updated_at
FROM agents
WHERE deleted_at IS NULL;

DROP TYPE IF EXISTS AGENT_COMMUNICATION_STYLE;
DROP TYPE IF EXISTS AGENT_RESPONSE_LENGTH;
DROP TYPE IF EXISTS AGENT_TONE;
