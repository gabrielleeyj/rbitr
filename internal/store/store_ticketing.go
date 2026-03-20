package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func (s *Store) GetTicketingConfig(ctx context.Context, tenantID string) (models.TicketingConfig, error) {
	var cfg models.TicketingConfig
	query := `SELECT tenant_id, provider, enabled, base_url, secret_ref, project_key, issue_type,
		auto_create, webhook_secret_ref, created_at, updated_at
		FROM rbitr.ticketing_config WHERE tenant_id = $1`
	row := s.db.QueryRowContext(ctx, query, tenantID)
	if err := row.Scan(
		&cfg.TenantID, &cfg.Provider, &cfg.Enabled, &cfg.BaseURL, &cfg.SecretRef,
		&cfg.ProjectKey, &cfg.IssueType, &cfg.AutoCreate, &cfg.WebhookSecretRef,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TicketingConfig{}, ErrNotFound
		}
		return models.TicketingConfig{}, err
	}
	return cfg, nil
}

func (s *Store) UpsertTicketingConfig(ctx context.Context, config *models.TicketingConfig) error {
	query := `INSERT INTO rbitr.ticketing_config
		(tenant_id, provider, enabled, base_url, secret_ref, project_key, issue_type,
		 auto_create, webhook_secret_ref, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT (tenant_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			enabled = EXCLUDED.enabled,
			base_url = EXCLUDED.base_url,
			secret_ref = EXCLUDED.secret_ref,
			project_key = EXCLUDED.project_key,
			issue_type = EXCLUDED.issue_type,
			auto_create = EXCLUDED.auto_create,
			webhook_secret_ref = EXCLUDED.webhook_secret_ref,
			updated_at = NOW()`
	_, err := s.db.ExecContext(ctx, query,
		config.TenantID, config.Provider, config.Enabled, config.BaseURL, config.SecretRef,
		config.ProjectKey, config.IssueType, config.AutoCreate, config.WebhookSecretRef,
	)
	return err
}

func (s *Store) InsertTicketLink(ctx context.Context, link *models.TicketLink) error {
	query := `INSERT INTO rbitr.ticket_links
		(ticket_link_id, tenant_id, approval_request_id, provider, external_key, external_url, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := s.db.ExecContext(ctx, query,
		link.TicketLinkID, link.TenantID, link.ApprovalRequestID,
		link.Provider, link.ExternalKey, link.ExternalURL, link.Status,
		link.CreatedAt, link.UpdatedAt,
	)
	return err
}

func (s *Store) GetTicketLinkByApproval(ctx context.Context, tenantID, approvalRequestID string) (models.TicketLink, error) {
	var link models.TicketLink
	query := `SELECT ticket_link_id, tenant_id, approval_request_id, provider, external_key, external_url, status, created_at, updated_at
		FROM rbitr.ticket_links WHERE tenant_id = $1 AND approval_request_id = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, approvalRequestID)
	if err := row.Scan(
		&link.TicketLinkID, &link.TenantID, &link.ApprovalRequestID,
		&link.Provider, &link.ExternalKey, &link.ExternalURL, &link.Status,
		&link.CreatedAt, &link.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TicketLink{}, ErrNotFound
		}
		return models.TicketLink{}, err
	}
	return link, nil
}

func (s *Store) GetTicketLinkByExternalKey(ctx context.Context, provider, externalKey string) (models.TicketLink, error) {
	var link models.TicketLink
	query := `SELECT ticket_link_id, tenant_id, approval_request_id, provider, external_key, external_url, status, created_at, updated_at
		FROM rbitr.ticket_links WHERE provider = $1 AND external_key = $2`
	row := s.db.QueryRowContext(ctx, query, provider, externalKey)
	if err := row.Scan(
		&link.TicketLinkID, &link.TenantID, &link.ApprovalRequestID,
		&link.Provider, &link.ExternalKey, &link.ExternalURL, &link.Status,
		&link.CreatedAt, &link.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TicketLink{}, ErrNotFound
		}
		return models.TicketLink{}, err
	}
	return link, nil
}

func (s *Store) ListTicketLinks(ctx context.Context, tenantID string, limit, offset int) ([]models.TicketLink, error) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	query := `SELECT ticket_link_id, tenant_id, approval_request_id, provider, external_key, external_url, status, created_at, updated_at
		FROM rbitr.ticket_links WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := s.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []models.TicketLink
	for rows.Next() {
		var link models.TicketLink
		if scanErr := rows.Scan(
			&link.TicketLinkID, &link.TenantID, &link.ApprovalRequestID,
			&link.Provider, &link.ExternalKey, &link.ExternalURL, &link.Status,
			&link.CreatedAt, &link.UpdatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) UpdateTicketLinkStatus(ctx context.Context, ticketLinkID, status string) error {
	query := `UPDATE rbitr.ticket_links SET status = $1, updated_at = $2 WHERE ticket_link_id = $3`
	result, err := s.db.ExecContext(ctx, query, status, time.Now().UTC(), ticketLinkID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
