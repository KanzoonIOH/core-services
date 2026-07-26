-- +goose Up
-- conversations.agent_id previously required a real agents(id). Orchestrators
-- share the same /chat/{id} route but their id is not an agent id, so their
-- conversations FK-failed and never appeared in chat history. Drop the FK so
-- the column can hold an agent OR an orchestrator id. Name resolution moves to
-- a LEFT JOIN against both tables in the conversation queries.
ALTER TABLE conversations DROP CONSTRAINT conversations_agent_id_fkey;

-- +goose Down
ALTER TABLE conversations
ADD CONSTRAINT conversations_agent_id_fkey
FOREIGN KEY (agent_id) REFERENCES agents (id) ON DELETE CASCADE;
