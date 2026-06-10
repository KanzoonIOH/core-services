-- +goose Up
CREATE TABLE conversation_analytics (
    conversation_id     String,
    agent_id            String,
    session_id          String,
    started_at          DateTime64(3, 'UTC'),
    ended_at            DateTime64(3, 'UTC'),
    resolution_ms       Int64,
    end_reason          LowCardinality(String),
    message_count       Int32,
    channel             LowCardinality(String),

    -- `intent` is the primary intent. `intents` keeps all detected intents and
    -- is what powers intent frequency analytics.
    intent              LowCardinality(String),
    intents             Array(String),
    topics              Array(String),
    sentiment           LowCardinality(String),
    sentiment_score     Float32,
    language            LowCardinality(String),

    is_resolved         Bool,
    escalation_reason   Nullable(String),
    first_response_ms   Nullable(Int64),
    user_satisfaction   Nullable(Int8),

    summary             String,
    keywords            Array(String),

    external_user_id    Nullable(String),
    tags                Array(String),
    model_version       LowCardinality(String),

    -- Conversation analytics occur when the conversation ends. Producers should
    -- set occurred_at = ended_at so time-series reflect conversation time, not
    -- delayed enrichment time.
    occurred_at         DateTime64(3, 'UTC'),
    created_at          DateTime64(3, 'UTC') DEFAULT now64(3),

    INDEX idx_agent_id agent_id TYPE set(10000) GRANULARITY 4,
    INDEX idx_occurred_at occurred_at TYPE minmax GRANULARITY 1
)
ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY conversation_id
TTL occurred_at + INTERVAL 5 YEAR;

-- +goose Down
DROP TABLE IF EXISTS conversation_analytics;
