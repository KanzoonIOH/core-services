-- +goose Up
CREATE TABLE webhook_messages (
    agent_id         String,
    conversation_id  String,
    status_code      Int32 COMMENT 'HTTP status from the webhook call; 0 means no HTTP response was received',
    response_time_ms Int64,
    is_success       Bool COMMENT 'Application-level success flag from the event producer',
    error            Nullable(String),
    occurred_at      DateTime64(3, 'UTC'),
    created_at       DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (agent_id, occurred_at, conversation_id)
TTL occurred_at + INTERVAL 180 DAY;

-- +goose Down
DROP TABLE IF EXISTS webhook_messages;
