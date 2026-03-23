package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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
