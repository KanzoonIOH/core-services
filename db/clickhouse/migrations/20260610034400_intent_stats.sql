-- +goose Up
CREATE VIEW intent_stats AS
SELECT
    agent_id,
    toDate(occurred_at) AS day,
    intent,
    count() AS total,
    countIf(is_resolved) AS resolved_count
FROM (
    SELECT
        agent_id,
        occurred_at,
        is_resolved,
        arrayJoin(arrayDistinct(arrayFilter(x -> x != '', if(length(intents) > 0, intents, [toString(intent)])))) AS intent
    FROM conversation_analytics FINAL
)
GROUP BY agent_id, day, intent;

-- +goose Down
DROP VIEW IF EXISTS intent_stats;
