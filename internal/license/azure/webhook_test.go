package azure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func newTestWebhookHandler(t *testing.T, mock *mockFulfillmentClient) *WebhookHandler {
	t.Helper()

	provider, err := NewProvider(mock, "", PlanPro)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	metMock := &mockMeteringClient{}
	reporter := NewReporter(metMock, "", PlanPro)

	return NewWebhookHandler(provider, reporter, mock)
}

func TestWebhookHandler_Landing_Success(t *testing.T) {
	mock := &mockFulfillmentClient{
		resolveFunc: func(_ context.Context, token string) (*ResolvedSubscription, error) {
			if token != "test-token-123" {
				t.Errorf("token = %q, want %q", token, "test-token-123")
			}
			return &ResolvedSubscription{
				SubscriptionID: "sub-resolved",
				PlanID:         PlanPro,
			}, nil
		},
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanPro, Status: StatusSubscribed}, nil
		},
	}

	handler := newTestWebhookHandler(t, mock)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/marketplace/azure/landing?token=test-token-123", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLanding(c)
	if err != nil {
		t.Fatalf("HandleLanding error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if mock.activateCallCount != 1 {
		t.Errorf("activateCallCount = %d, want 1", mock.activateCallCount)
	}

	if mock.lastActivatedSubID != "sub-resolved" {
		t.Errorf("activated sub = %q, want %q", mock.lastActivatedSubID, "sub-resolved")
	}
}

func TestWebhookHandler_Landing_MissingToken(t *testing.T) {
	mock := &mockFulfillmentClient{}
	handler := newTestWebhookHandler(t, mock)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/marketplace/azure/landing", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLanding(c)
	if err != nil {
		t.Fatalf("HandleLanding error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWebhookHandler_Landing_ResolveFailure(t *testing.T) {
	mock := &mockFulfillmentClient{
		resolveFunc: func(_ context.Context, _ string) (*ResolvedSubscription, error) {
			return nil, errors.New("Azure API error")
		},
	}
	handler := newTestWebhookHandler(t, mock)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/marketplace/azure/landing?token=bad-token", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLanding(c)
	if err != nil {
		t.Fatalf("HandleLanding error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestWebhookHandler_Webhook_ChangePlan(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanEnterprise, Status: StatusSubscribed}, nil
		},
	}
	handler := newTestWebhookHandler(t, mock)

	body := `{
		"action": "ChangePlan",
		"subscriptionId": "sub-123",
		"planId": "enterprise",
		"id": "op-1"
	}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/azure/webhook", strings.NewReader(body))
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

func TestWebhookHandler_Webhook_Suspend(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanPro, Status: StatusSuspended}, nil
		},
	}
	handler := newTestWebhookHandler(t, mock)

	body := `{
		"action": "Suspend",
		"subscriptionId": "sub-123",
		"id": "op-2"
	}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/azure/webhook", strings.NewReader(body))
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

func TestWebhookHandler_Webhook_Reinstate(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanPro, Status: StatusSubscribed}, nil
		},
	}
	handler := newTestWebhookHandler(t, mock)

	body := `{"action": "Reinstate", "subscriptionId": "sub-123", "id": "op-3"}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/azure/webhook", strings.NewReader(body))
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

func TestWebhookHandler_Webhook_Unsubscribe(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanPro, Status: StatusUnsubscribed}, nil
		},
	}
	handler := newTestWebhookHandler(t, mock)

	body := `{"action": "Unsubscribe", "subscriptionId": "sub-123", "id": "op-4"}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/azure/webhook", strings.NewReader(body))
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

func TestWebhookHandler_Webhook_UnknownAction(t *testing.T) {
	mock := &mockFulfillmentClient{}
	handler := newTestWebhookHandler(t, mock)

	body := `{"action": "Unknown", "subscriptionId": "sub-123"}`

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/marketplace/azure/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleWebhook(c)
	if err != nil {
		t.Fatalf("HandleWebhook error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (always ack webhook)", rec.Code, http.StatusOK)
	}
}
