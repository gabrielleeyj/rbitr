-- +goose Up

CREATE TABLE IF NOT EXISTS rbitr.setup_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    state TEXT NOT NULL,
    last_error TEXT NULL,
    actor_token_fingerprint TEXT NULL,
    actor_ip TEXT NULL,
    last_request_id TEXT NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT setup_state_enum_chk CHECK (state IN ('not_started', 'in_progress', 'completed', 'failed'))
);

INSERT INTO rbitr.setup_state (singleton, state, updated_at)
VALUES (TRUE, 'not_started', NOW())
ON CONFLICT (singleton) DO NOTHING;

CREATE TABLE IF NOT EXISTS rbitr.setup_initialize_idempotency (
    idempotency_key TEXT PRIMARY KEY,
    payload_hash TEXT NOT NULL,
    response_json JSONB NOT NULL,
    token_fingerprint TEXT NULL,
    client_ip TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_setup_initialize_idempotency_created_at
ON rbitr.setup_initialize_idempotency (created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS rbitr.idx_setup_initialize_idempotency_created_at;
DROP TABLE IF EXISTS rbitr.setup_initialize_idempotency;
DROP TABLE IF EXISTS rbitr.setup_state;
