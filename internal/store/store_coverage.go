package store

import (
	"context"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

// ListFallbackHitPairs returns aggregated (tool, action, risk) pairs whose
// decisions were made by one of the given fallback rule ids since the cutoff
// time, ordered by most-hit first. These represent endpoints with ambiguous
// permissions that were exercised by real traffic.
func (s *Store) ListFallbackHitPairs(ctx context.Context, tenantID string, ruleIDs []string, since time.Time, limit int) ([]models.CoverageFallbackHit, error) {
	query := `SELECT tool_id, action_type, action_risk, decision, rule_id,
			COUNT(*) AS hit_count, MAX(created_at) AS last_seen
		FROM rbitr.action_decisions
		WHERE tenant_id = $1 AND rule_id = ANY($2) AND created_at >= $3
		GROUP BY tool_id, action_type, action_risk, decision, rule_id
		ORDER BY hit_count DESC, last_seen DESC
		LIMIT $4`
	rows, err := s.db.QueryContext(ctx, query, tenantID, StringArray(ruleIDs), since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []models.CoverageFallbackHit
	for rows.Next() {
		var hit models.CoverageFallbackHit
		if err := rows.Scan(&hit.ToolID, &hit.ActionType, &hit.ActionRisk, &hit.Decision, &hit.RuleID, &hit.HitCount, &hit.LastSeen); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

// ListUnusedActiveTools returns the ids of active (non-archived) tools that have
// no recorded decisions at all. These are registered endpoints whose permissions
// have never been exercised and so remain unconfigured.
func (s *Store) ListUnusedActiveTools(ctx context.Context, tenantID string) ([]string, error) {
	query := `SELECT t.tool_id
		FROM rbitr.tools t
		WHERE t.tenant_id = $1
		  AND t.archived_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM rbitr.action_decisions d
		      WHERE d.tenant_id = t.tenant_id AND d.tool_id = t.tool_id
		  )
		ORDER BY t.tool_id`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var toolIDs []string
	for rows.Next() {
		var toolID string
		if err := rows.Scan(&toolID); err != nil {
			return nil, err
		}
		toolIDs = append(toolIDs, toolID)
	}
	return toolIDs, rows.Err()
}
