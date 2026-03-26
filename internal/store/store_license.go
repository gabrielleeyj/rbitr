package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrNoTrialStarted is returned when no trial has been started.
var ErrNoTrialStarted = errors.New("no trial started")

func (s *Store) InsertLicenseHistory(ctx context.Context, tier string, keyVersion int, licensee, email string, expiresAt time.Time, fingerprint string) error {
	query := `INSERT INTO rbitr.license_history (tier, key_version, licensee, email, expires_at, fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := s.db.ExecContext(ctx, query, tier, keyVersion, licensee, email, expiresAt, fingerprint)
	return err
}

func (s *Store) GetLatestLicenseHistory(ctx context.Context) (LicenseHistoryRecord, error) {
	var rec LicenseHistoryRecord
	query := `SELECT id, tier, key_version, licensee, email, expires_at, activated_at, fingerprint
		FROM rbitr.license_history ORDER BY activated_at DESC LIMIT 1`
	row := s.db.QueryRowContext(ctx, query)
	err := row.Scan(&rec.ID, &rec.Tier, &rec.KeyVersion, &rec.Licensee, &rec.Email, &rec.ExpiresAt, &rec.ActivatedAt, &rec.Fingerprint)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LicenseHistoryRecord{}, ErrNotFound
		}
		return LicenseHistoryRecord{}, err
	}
	return rec, nil
}

// GetEarliestTrialStartDate returns the earliest trial_started_at from all tenants.
// This represents the application-wide trial start date.
func (s *Store) GetEarliestTrialStartDate(ctx context.Context) (*time.Time, error) {
	query := `SELECT trial_started_at FROM rbitr.tenant_config
		WHERE trial_started_at IS NOT NULL
		ORDER BY trial_started_at ASC LIMIT 1`
	row := s.db.QueryRowContext(ctx, query)
	var trialStartedAt sql.NullTime
	err := row.Scan(&trialStartedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoTrialStarted
		}
		return nil, err
	}
	if !trialStartedAt.Valid {
		return nil, ErrNoTrialStarted
	}
	return &trialStartedAt.Time, nil
}

// HasTrialLicenseBeenUsed checks if a trial license key has ever been uploaded.
// Returns true if any license with tier="trial" exists in license_history.
func (s *Store) HasTrialLicenseBeenUsed(ctx context.Context) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM rbitr.license_history WHERE tier = 'trial' LIMIT 1)`
	row := s.db.QueryRowContext(ctx, query)
	var exists bool
	err := row.Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
