-- +goose Up
CREATE VIEW topic_stats AS
SELECT
    agent_id,
    toDate(occurred_at) AS day,
    topic,
    count() AS total,
    countIf(is_resolved) AS resolved_count
FROM (
    SELECT
        agent_id,
        occurred_at,
        is_resolved,
        arrayJoin(arrayDistinct(arrayFilter(x -> x != '', topics))) AS topic
    FROM conversation_analytics FINAL
)
GROUP BY agent_id, day, topic;

-- +goose Down
DROP VIEW IF EXISTS topic_stats;
