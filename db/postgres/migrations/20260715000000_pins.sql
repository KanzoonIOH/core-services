-- +goose Up
-- App-wide pins. A pin is a per-user shortcut to any pinnable entity
-- (dashboard, agent, orchestrator, chat). It exists purely for fast navigation
-- and renders in a "Pinned" section at the top of the sidebar.
--
-- We store only the reference (entity_type + entity_id), never a label snapshot:
-- the sidebar resolves the current name/image live from each entity's view, so
-- renaming an agent or dashboard updates its pin automatically. A pin whose
-- target was deleted resolves to NULL and is filtered out at read time.
--
-- entity_type is a free-form text guarded by a CHECK so new pinnable types can
-- be added with a one-line migration instead of an enum ALTER.
CREATE TABLE pins (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    -- Manual sidebar ordering; lower sorts first. Appended at max+1 on insert.
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pins_entity_type_check
    CHECK (entity_type IN ('dashboard', 'agent', 'orchestrator', 'chat'))
);

-- One pin per (user, entity). Re-pinning is a no-op (ON CONFLICT in the query).
CREATE UNIQUE INDEX idx_pins_unique
ON pins (user_id, entity_type, entity_id);

CREATE INDEX idx_pins_user_id ON pins (user_id);

-- +goose Down
DROP TABLE IF EXISTS pins;
