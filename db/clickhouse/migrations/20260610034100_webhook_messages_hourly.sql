-- +goose Up
CREATE TABLE webhook_messages_hourly (
    agent_id             String,
    bucket               DateTime('UTC'),
    total                AggregateFunction(count),
    success_count        AggregateFunction(countIf, Bool),
    failure_count        AggregateFunction(countIf, Bool),
    success_resp_time_sum AggregateFunction(sum, Int64),
    resp_time_pct        AggregateFunction(quantilesTDigest(0.5, 0.9, 0.95, 0.99), Int64),
    uniq_convos          AggregateFunction(uniq, String)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (agent_id, bucket)
TTL bucket + INTERVAL 5 YEAR;

CREATE MATERIALIZED VIEW webhook_messages_hourly_mv
TO webhook_messages_hourly AS
SELECT
    agent_id,
    toStartOfHour(occurred_at) AS bucket,
    countState() AS total,
    countIfState(is_success) AS success_count,
    countIfState(NOT is_success) AS failure_count,
    sumState(if(is_success, response_time_ms, 0)) AS success_resp_time_sum,
    quantilesTDigestState(0.5, 0.9, 0.95, 0.99)(response_time_ms) AS resp_time_pct,
    uniqState(conversation_id) AS uniq_convos
FROM webhook_messages
GROUP BY agent_id, bucket;

-- Safe for a reset/fresh database. Do not run this backfill concurrently with
-- live writes on an already-populated webhook_messages table.
INSERT INTO webhook_messages_hourly
SELECT
    agent_id,
    toStartOfHour(occurred_at) AS bucket,
    countState() AS total,
    countIfState(is_success) AS success_count,
    countIfState(NOT is_success) AS failure_count,
    sumState(if(is_success, response_time_ms, 0)) AS success_resp_time_sum,
    quantilesTDigestState(0.5, 0.9, 0.95, 0.99)(response_time_ms) AS resp_time_pct,
    uniqState(conversation_id) AS uniq_convos
FROM webhook_messages
GROUP BY agent_id, bucket;

-- +goose Down
DROP VIEW IF EXISTS webhook_messages_hourly_mv;
DROP TABLE IF EXISTS webhook_messages_hourly;
