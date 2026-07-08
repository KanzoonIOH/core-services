-- +goose Up
-- Single-row table holding app-wide config applied to every agent's webhook.
-- id is a fixed boolean pinned to true, so there can only ever be one row.
CREATE TABLE global_config (
    id BOOLEAN PRIMARY KEY DEFAULT true,
    agent_name TEXT NOT NULL DEFAULT '',
    industry_description TEXT NOT NULL DEFAULT '',
    guardrail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT global_config_singleton CHECK (id)
);

-- Seed the single row so GET always has something to return.
INSERT INTO global_config (id) VALUES (true);

-- +goose Down
DROP TABLE IF EXISTS global_config;
