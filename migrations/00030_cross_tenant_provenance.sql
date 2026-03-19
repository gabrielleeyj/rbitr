-- +goose Up

-- Add source_decision_id column for cross-tenant request provenance chain.
-- Links an action decision to the upstream decision that triggered it.
ALTER TABLE rbitr.action_decisions
    ADD COLUMN IF NOT EXISTS source_decision_id TEXT;

-- Index for looking up downstream decisions from a source.
CREATE INDEX IF NOT EXISTS idx_action_decisions_source_decision_id
    ON rbitr.action_decisions (source_decision_id)
    WHERE source_decision_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS rbitr.idx_action_decisions_source_decision_id;
ALTER TABLE rbitr.action_decisions DROP COLUMN IF EXISTS source_decision_id;
