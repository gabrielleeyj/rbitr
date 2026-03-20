package ticketing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type mockResolver struct {
	secrets map[string]string
}

func (m *mockResolver) Resolve(_ context.Context, ref string) (string, error) {
	if v, ok := m.secrets[ref]; ok {
		return v, nil
	}
	return "", store.ErrNotFound
}

func TestService_OnApprovalCreated_AutoCreateDisabled(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTicketingConfig", context.Background(), "t1").Return(models.TicketingConfig{
		Enabled:    true,
		AutoCreate: false,
	}, nil)

	svc := NewService(storeMock, &mockResolver{})
	approval := &models.ApprovalRequest{
		ApprovalRequestID: "ar-1",
		TenantID:          "t1",
	}

	svc.OnApprovalCreated(context.Background(), "t1", approval)
	storeMock.AssertNotCalled(t, "InsertTicketLink")
}

func TestService_OnApprovalCreated_NotConfigured(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTicketingConfig", context.Background(), "t1").Return(models.TicketingConfig{}, store.ErrNotFound)

	svc := NewService(storeMock, &mockResolver{})
	svc.OnApprovalCreated(context.Background(), "t1", &models.ApprovalRequest{})
}

func TestMapApprovalToTicketStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"approved", TicketStatusResolved},
		{"denied", TicketStatusClosed},
		{"revoked", TicketStatusClosed},
		{"expired", TicketStatusClosed},
		{"unknown", TicketStatusInProgress},
	}
	for _, tt := range tests {
		got := mapApprovalToTicketStatus(tt.input)
		if got != tt.expected {
			t.Errorf("mapApprovalToTicketStatus(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildDescription(t *testing.T) {
	approval := &models.ApprovalRequest{
		ApprovalRequestID: "ar-123",
		TenantID:          "t1",
		AgentID:           "agent-1",
		ToolID:            "tool-1",
		ActionType:        "file.write",
		Risk:              "HIGH",
		ActionSummary:     "Writing config file",
		ExpiresAt:         time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
	}
	desc := buildDescription(approval)
	if desc == "" {
		t.Fatal("expected non-empty description")
	}
	expectedParts := []string{
		"ar-123", "t1", "agent-1", "tool-1", "file.write", "HIGH", "Writing config file", "2026-03-20T12:00:00Z",
	}
	for _, part := range expectedParts {
		if !strings.Contains(desc, part) {
			t.Errorf("description missing %q", part)
		}
	}
}
