-- +goose Up
CREATE TABLE webhook_messages_hourly (
    agent_id      String,
    bucket        DateTime('UTC'),
    total         AggregateFunction(count),
    success_count AggregateFunction(countIf, Bool),
    resp_time_avg AggregateFunction(avg, Int64),
    uniq_convos   AggregateFunction(uniq, String)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (agent_id, bucket);

CREATE MATERIALIZED VIEW webhook_messages_hourly_mv
TO webhook_messages_hourly AS
SELECT
    agent_id,
    toStartOfHour(occurred_at)  AS bucket,
    countState()                AS total,
    countIfState(is_success)    AS success_count,
    avgState(response_time_ms)  AS resp_time_avg,
    uniqState(conversation_id)  AS uniq_convos
FROM webhook_messages
GROUP BY agent_id, bucket;

INSERT INTO webhook_messages_hourly
SELECT
    agent_id,
    toStartOfHour(occurred_at)  AS bucket,
    countState()                AS total,
    countIfState(is_success)    AS success_count,
    avgState(response_time_ms)  AS resp_time_avg,
    uniqState(conversation_id)  AS uniq_convos
FROM webhook_messages
GROUP BY agent_id, bucket;

-- +goose Down
DROP VIEW IF EXISTS webhook_messages_hourly_mv;
DROP TABLE IF EXISTS webhook_messages_hourly;
