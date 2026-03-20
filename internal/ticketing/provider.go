package ticketing

import "context"

// TicketProvider defines the interface for ticketing system integrations.
type TicketProvider interface {
	CreateTicket(ctx context.Context, req *CreateTicketRequest) (CreateTicketResult, error)
	UpdateTicket(ctx context.Context, req *UpdateTicketRequest) error
	Name() string
}

type CreateTicketRequest struct {
	ProjectKey  string
	IssueType   string
	Summary     string
	Description string
	Priority    string
	Labels      []string
	Metadata    map[string]string
}

type CreateTicketResult struct {
	ExternalKey string
	ExternalURL string
}

type UpdateTicketRequest struct {
	ExternalKey string
	Status      string
	Comment     string
}
