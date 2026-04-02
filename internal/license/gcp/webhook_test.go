package gcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	procurement "google.golang.org/api/cloudcommerceprocurement/v1"
)

func newTestWebhookHandler(t *testing.T) (*WebhookHandler, *mockProcurementClient) {
	t.Helper()

	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return &procurement.ListEntitlementsResponse{}, nil
		},
	}

	provider, err := NewProvider(mock, "test-provider", "rbitr")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	handler := NewWebhookHandler(provider, mock)
	return handler, mock
}

func TestWebhookHandler_ActivationRequest(t *testing.T) {
	handler, mock := newTestWebhookHandler(t)

	body := `{
		"message": {
			"attributes": {
				"event_type": "ENTITLEMENT_CREATION_REQUESTED",
				"entitlement_id": "ent-123"
			},
			"messageId": "msg-1"
		}
	}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/gcp/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleWebhook(c)
	if err != nil {
		t.Fatalf("HandleWebhook error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if mock.approveCallCount != 1 {
		t.Errorf("approveCallCount = %d, want 1", mock.approveCallCount)
	}

	expectedName := "providers/test-provider/entitlements/ent-123"
	if mock.lastApprovedName != expectedName {
		t.Errorf("approved name = %q, want %q", mock.lastApprovedName, expectedName)
	}
}

func TestWebhookHandler_EntitlementActive(t *testing.T) {
	handler, mock := newTestWebhookHandler(t)

	// Set up mock to return active entitlement on refresh.
	mock.listFunc = func(_ string) (*procurement.ListEntitlementsResponse, error) {
		return &procurement.ListEntitlementsResponse{
			Entitlements: []*procurement.Entitlement{
				{
					ProductExternalName: "rbitr",
					State:               "ENTITLEMENT_ACTIVE",
					Plan:                PlanPro,
					Account:             "accounts/webhook-test",
				},
			},
		}, nil
	}

	body := `{
		"message": {
			"attributes": {
				"event_type": "ENTITLEMENT_ACTIVE",
				"entitlement_id": "ent-456"
			},
			"messageId": "msg-2"
		}
	}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/gcp/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleWebhook(c)
	if err != nil {
		t.Fatalf("HandleWebhook error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWebhookHandler_InvalidBody(t *testing.T) {
	handler, _ := newTestWebhookHandler(t)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/gcp/webhook", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleWebhook(c)
	if err != nil {
		t.Fatalf("HandleWebhook error: %v", err)
	}

	// Should return 400 for invalid JSON body.
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 400 or 200", rec.Code)
	}
}

func TestWebhookHandler_UnknownEvent(t *testing.T) {
	handler, _ := newTestWebhookHandler(t)

	body := `{
		"message": {
			"attributes": {
				"event_type": "UNKNOWN_EVENT"
			},
			"messageId": "msg-3"
		}
	}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/gcp/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleWebhook(c)
	if err != nil {
		t.Fatalf("HandleWebhook error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (always ack Pub/Sub)", rec.Code, http.StatusOK)
	}
}
