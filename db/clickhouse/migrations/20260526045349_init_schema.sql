-- +goose Up

-- Per-message analytics: traffic, response time, success rate
CREATE TABLE webhook_messages (
    agent_id         String,
    conversation_id  String,
    status_code      Int32,
    response_time_ms Int64,
    is_success       Bool,
    error            Nullable(String),
    occurred_at      DateTime64(3, 'UTC'),
    created_at       DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (agent_id, conversation_id, occurred_at);

-- Per-conversation analytics: total convos, escalation rate, resolution time
CREATE TABLE conversation_events (
    agent_id          String,
    conversation_id   String,
    -- 'resolved' | 'escalated' | 'timed_out'
    end_reason        LowCardinality(String),
    escalation_reason Nullable(String),
    started_at        DateTime64(3, 'UTC'),
    ended_at          DateTime64(3, 'UTC'),
    resolution_ms     Int64,
    occurred_at       DateTime64(3, 'UTC'),
    created_at        DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (agent_id, conversation_id, occurred_at);

-- +goose Down
DROP TABLE IF EXISTS conversation_events;
DROP TABLE IF EXISTS webhook_messages;
