-- License activation audit trail.
-- Stores metadata about each license key activation (not the key itself).
CREATE TABLE IF NOT EXISTS rbitr.license_history (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tier         TEXT NOT NULL,
    key_version  INTEGER NOT NULL,
    licensee     TEXT NOT NULL,
    email        TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    fingerprint  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_license_history_fingerprint
    ON rbitr.license_history(fingerprint);

CREATE INDEX IF NOT EXISTS idx_license_history_activated_at
    ON rbitr.license_history(activated_at DESC);
