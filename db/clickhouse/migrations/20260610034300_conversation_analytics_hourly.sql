-- +goose Up
CREATE VIEW conversation_analytics_hourly AS
SELECT
    agent_id,
    toStartOfHour(occurred_at) AS bucket,
    count() AS total,
    countIf(is_resolved) AS resolved_count,
    countIf(end_reason = 'escalated') AS escalated_count,
    countIf(end_reason = 'timed_out') AS timed_out_count,
    countIf(end_reason = 'human_intercepted') AS intercepted_count,
    sum(resolution_ms) AS resolution_ms_sum,
    countIf(sentiment = 'positive') AS positive_count,
    countIf(sentiment = 'neutral') AS neutral_count,
    countIf(sentiment = 'negative') AS negative_count,
    countIf(sentiment NOT IN ('positive', 'neutral', 'negative')) AS other_sentiment_count,
    uniqExactIf(assumeNotNull(external_user_id), isNotNull(external_user_id)) AS uniq_users,
    sum(message_count) AS message_count_sum,
    sumIf(assumeNotNull(first_response_ms), isNotNull(first_response_ms)) AS first_response_ms_sum,
    countIf(isNotNull(first_response_ms)) AS first_response_count,
    sumIf(assumeNotNull(user_satisfaction), isNotNull(user_satisfaction)) AS user_satisfaction_sum,
    countIf(isNotNull(user_satisfaction)) AS user_satisfaction_count
FROM conversation_analytics FINAL
GROUP BY agent_id, bucket;

-- +goose Down
DROP VIEW IF EXISTS conversation_analytics_hourly;
