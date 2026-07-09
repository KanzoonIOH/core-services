-- +goose Up
CREATE TABLE audit_logs (
    occurred_at DateTime64(3, 'UTC') COMMENT 'when the action happened',
    user_id     String COMMENT 'actor user id; empty for api-key/unauthenticated',
    role        String,
    auth_method String COMMENT 'jwt | api_key | empty',
    action      LowCardinality(String) COMMENT 'CREATE | UPDATE | DELETE',
    menu        LowCardinality(String) COMMENT 'feature/resource, e.g. agents',
    method      LowCardinality(String) COMMENT 'raw HTTP method',
    path        String,
    status      Int32 COMMENT 'HTTP response status',
    created_at  DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (occurred_at, user_id, menu)
TTL occurred_at + INTERVAL 365 DAY;

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
