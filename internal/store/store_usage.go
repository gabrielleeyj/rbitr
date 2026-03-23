package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const maxUsageMeterListLimit = 24

// IncrementUsageMeter atomically increments the action count for the given
// tenant and period (YYYY-MM). Returns the new count after increment.
func (s *Store) IncrementUsageMeter(ctx context.Context, tenantID, period string) (int64, error) {
	query := `INSERT INTO rbitr.usage_meters (tenant_id, period, action_count, updated_at)
		VALUES ($1, $2, 1, now())
		ON CONFLICT (tenant_id, period)
		DO UPDATE SET action_count = rbitr.usage_meters.action_count + 1, updated_at = now()
		RETURNING action_count`

	var count int64
	err := s.db.QueryRowContext(ctx, query, tenantID, period).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: increment usage meter: %w", err)
	}
	return count, nil
}

// GetUsageMeter returns the current action count for the given tenant and period.
// Returns 0 if no meter exists for the period.
func (s *Store) GetUsageMeter(ctx context.Context, tenantID, period string) (int64, error) {
	query := `SELECT action_count FROM rbitr.usage_meters WHERE tenant_id = $1 AND period = $2`

	var count int64
	err := s.db.QueryRowContext(ctx, query, tenantID, period).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("store: get usage meter: %w", err)
	}
	return count, nil
}

// ListUsageMeters returns the most recent usage meter records for a tenant,
// ordered by period descending. Limit is clamped to 24 months.
func (s *Store) ListUsageMeters(ctx context.Context, tenantID string, limit int) ([]UsageMeterRecord, error) {
	if limit <= 0 || limit > maxUsageMeterListLimit {
		limit = maxUsageMeterListLimit
	}

	query := `SELECT tenant_id, period, action_count, updated_at
		FROM rbitr.usage_meters
		WHERE tenant_id = $1
		ORDER BY period DESC
		LIMIT $2`

	rows, err := s.db.QueryContext(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list usage meters: %w", err)
	}
	defer rows.Close()

	var records []UsageMeterRecord
	for rows.Next() {
		var rec UsageMeterRecord
		if scanErr := rows.Scan(&rec.TenantID, &rec.Period, &rec.ActionCount, &rec.UpdatedAt); scanErr != nil {
			return nil, fmt.Errorf("store: list usage meters scan: %w", scanErr)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}
