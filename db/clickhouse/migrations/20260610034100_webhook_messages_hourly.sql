-- +goose Up
CREATE TABLE webhook_messages_hourly (
    agent_id             String,
    bucket               DateTime('UTC'),
    total                AggregateFunction(count),
    success_count        AggregateFunction(countIf, Bool),
    failure_count        AggregateFunction(countIf, Bool),
    resp_time_pct        AggregateFunction(quantilesTDigest(0.5, 0.95, 0.99), Int64),
    success_2xx_count    AggregateFunction(countIf, Bool),
    redirect_3xx_count   AggregateFunction(countIf, Bool),
    client_4xx_count     AggregateFunction(countIf, Bool),
    server_5xx_count     AggregateFunction(countIf, Bool),
    gateway_502_count    AggregateFunction(countIf, Bool),
    no_response_count    AggregateFunction(countIf, Bool),
    other_status_count   AggregateFunction(countIf, Bool),
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
    quantilesTDigestState(0.5, 0.95, 0.99)(response_time_ms) AS resp_time_pct,
    countIfState(status_code >= 200 AND status_code < 300) AS success_2xx_count,
    countIfState(status_code >= 300 AND status_code < 400) AS redirect_3xx_count,
    countIfState(status_code >= 400 AND status_code < 500) AS client_4xx_count,
    countIfState(status_code >= 500 AND status_code < 600) AS server_5xx_count,
    countIfState(status_code = 502) AS gateway_502_count,
    countIfState(status_code = 0) AS no_response_count,
    countIfState(status_code != 0 AND (status_code < 200 OR status_code >= 600)) AS other_status_count,
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
    quantilesTDigestState(0.5, 0.95, 0.99)(response_time_ms) AS resp_time_pct,
    countIfState(status_code >= 200 AND status_code < 300) AS success_2xx_count,
    countIfState(status_code >= 300 AND status_code < 400) AS redirect_3xx_count,
    countIfState(status_code >= 400 AND status_code < 500) AS client_4xx_count,
    countIfState(status_code >= 500 AND status_code < 600) AS server_5xx_count,
    countIfState(status_code = 502) AS gateway_502_count,
    countIfState(status_code = 0) AS no_response_count,
    countIfState(status_code != 0 AND (status_code < 200 OR status_code >= 600)) AS other_status_count,
    uniqState(conversation_id) AS uniq_convos
FROM webhook_messages
GROUP BY agent_id, bucket;

-- +goose Down
DROP VIEW IF EXISTS webhook_messages_hourly_mv;
DROP TABLE IF EXISTS webhook_messages_hourly;
