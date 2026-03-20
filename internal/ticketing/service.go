package ticketing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

// Service orchestrates ticket creation and updates across providers.
type Service struct {
	Store    store.StoreAPI
	Resolver notifications.SecretResolver
}

func NewService(st store.StoreAPI, resolver notifications.SecretResolver) *Service {
	return &Service{Store: st, Resolver: resolver}
}

// OnApprovalCreated creates a ticket if auto_create is enabled for the tenant.
func (s *Service) OnApprovalCreated(ctx context.Context, tenantID string, approval *models.ApprovalRequest) {
	cfg, err := s.Store.GetTicketingConfig(ctx, tenantID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("ticketing: get config for %s: %v", tenantID, err)
		}
		return
	}
	if !cfg.Enabled || !cfg.AutoCreate {
		return
	}

	provider, providerErr := s.buildProvider(ctx, &cfg)
	if providerErr != nil {
		log.Printf("ticketing: build provider for %s: %v", tenantID, providerErr)
		return
	}

	priority := MapPriority(cfg.Provider, approval.Risk)
	description := buildDescription(approval)

	result, createErr := provider.CreateTicket(ctx, &CreateTicketRequest{
		ProjectKey:  cfg.ProjectKey,
		IssueType:   cfg.IssueType,
		Summary:     fmt.Sprintf("[rbitr] Approval required: %s (%s)", approval.ActionType, approval.ApprovalRequestID),
		Description: description,
		Priority:    priority,
		Labels:      []string{"rbitr", "approval"},
		Metadata: map[string]string{
			"approval_request_id": approval.ApprovalRequestID,
			"tenant_id":           tenantID,
		},
	})
	if createErr != nil {
		log.Printf("ticketing: create ticket for %s/%s: %v", tenantID, approval.ApprovalRequestID, createErr)
		return
	}

	now := time.Now().UTC()
	link := &models.TicketLink{
		TicketLinkID:      "tl_" + uuid.NewString(),
		TenantID:          tenantID,
		ApprovalRequestID: approval.ApprovalRequestID,
		Provider:          cfg.Provider,
		ExternalKey:       result.ExternalKey,
		ExternalURL:       result.ExternalURL,
		Status:            TicketStatusOpen,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if insertErr := s.Store.InsertTicketLink(ctx, link); insertErr != nil {
		log.Printf("ticketing: insert link for %s/%s: %v", tenantID, approval.ApprovalRequestID, insertErr)
	}
}

// OnApprovalDecided updates the linked ticket when an approval is approved, denied, or revoked.
func (s *Service) OnApprovalDecided(ctx context.Context, tenantID, approvalID, status, decidedBy, comment string) {
	s.updateTicket(ctx, tenantID, approvalID, status, fmt.Sprintf("Decision: %s by %s. %s", status, decidedBy, comment))
}

// OnApprovalExpired updates the linked ticket when an approval expires.
func (s *Service) OnApprovalExpired(ctx context.Context, tenantID, approvalID string) {
	s.updateTicket(ctx, tenantID, approvalID, "expired", "Approval expired in rbitr.")
}

func (s *Service) updateTicket(ctx context.Context, tenantID, approvalID, status, comment string) {
	link, err := s.Store.GetTicketLinkByApproval(ctx, tenantID, approvalID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("ticketing: get link for %s/%s: %v", tenantID, approvalID, err)
		}
		return
	}

	cfg, err := s.Store.GetTicketingConfig(ctx, tenantID)
	if err != nil {
		log.Printf("ticketing: get config for update %s: %v", tenantID, err)
		return
	}
	if !cfg.Enabled {
		return
	}

	provider, providerErr := s.buildProvider(ctx, &cfg)
	if providerErr != nil {
		log.Printf("ticketing: build provider for update %s: %v", tenantID, providerErr)
		return
	}

	ticketStatus := mapApprovalToTicketStatus(status)
	if updateErr := provider.UpdateTicket(ctx, &UpdateTicketRequest{
		ExternalKey: link.ExternalKey,
		Status:      ticketStatus,
		Comment:     comment,
	}); updateErr != nil {
		log.Printf("ticketing: update ticket %s: %v", link.ExternalKey, updateErr)
		return
	}

	if statusErr := s.Store.UpdateTicketLinkStatus(ctx, link.TicketLinkID, ticketStatus); statusErr != nil {
		log.Printf("ticketing: update link status %s: %v", link.TicketLinkID, statusErr)
	}
}

func (s *Service) buildProvider(ctx context.Context, cfg *models.TicketingConfig) (TicketProvider, error) {
	apiToken, err := s.Resolver.Resolve(ctx, cfg.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("resolve secret: %w", err)
	}

	switch cfg.Provider {
	case ProviderJira:
		email := parseJiraAuth(apiToken)
		return NewJiraProvider(cfg.BaseURL, apiToken, email), nil
	case ProviderServiceNow:
		return NewServiceNowProvider(cfg.BaseURL, apiToken), nil
	case ProviderLinear:
		return NewLinearProvider(apiToken), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func parseJiraAuth(token string) string {
	if strings.Contains(token, ":") {
		parts := strings.SplitN(token, ":", 2) //nolint:mnd // email:token format
		return parts[0]
	}
	return ""
}

func buildDescription(approval *models.ApprovalRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Approval Request: %s\n", approval.ApprovalRequestID)
	fmt.Fprintf(&b, "Tenant: %s\n", approval.TenantID)
	fmt.Fprintf(&b, "Agent: %s\n", approval.AgentID)
	fmt.Fprintf(&b, "Tool: %s\n", approval.ToolID)
	fmt.Fprintf(&b, "Action: %s\n", approval.ActionType)
	fmt.Fprintf(&b, "Risk: %s\n", approval.Risk)
	if approval.ActionSummary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", approval.ActionSummary)
	}
	fmt.Fprintf(&b, "Expires: %s\n", approval.ExpiresAt.Format(time.RFC3339))
	return b.String()
}

func mapApprovalToTicketStatus(approvalStatus string) string {
	switch strings.ToLower(approvalStatus) {
	case "approved":
		return TicketStatusResolved
	case "denied", "revoked":
		return TicketStatusClosed
	case "expired":
		return TicketStatusClosed
	default:
		return TicketStatusInProgress
	}
}
