-- +goose Up
ALTER TABLE rbitr.action_decisions
    ADD COLUMN decision_version TEXT NOT NULL DEFAULT 'v1',
    ADD COLUMN decision_risk TEXT NOT NULL DEFAULT 'UNKNOWN',
    ADD COLUMN rule_priority INT NOT NULL DEFAULT 0,
    ADD COLUMN reasons JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN constraints JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';

UPDATE rbitr.action_decisions
SET decision_risk = action_risk,
    reasons = jsonb_build_array(jsonb_build_object('code', 'LEGACY', 'message', reason)),
    constraints = '{}'::jsonb
WHERE decision_risk = 'UNKNOWN';

-- +goose Down
ALTER TABLE rbitr.action_decisions
    DROP COLUMN IF EXISTS decision_version,
    DROP COLUMN IF EXISTS decision_risk,
    DROP COLUMN IF EXISTS rule_priority,
    DROP COLUMN IF EXISTS reasons,
    DROP COLUMN IF EXISTS constraints,
    DROP COLUMN IF EXISTS tags;
