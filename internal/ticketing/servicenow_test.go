package ticketing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceNowProvider_CreateTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/now/table/incident" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header")
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["short_description"] != "Test ticket" {
			t.Errorf("unexpected summary: %s", body["short_description"])
		}
		if body["assignment_group"] != "IT-OPS" {
			t.Errorf("unexpected assignment group: %s", body["assignment_group"])
		}

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]string{
				"sys_id": "abc123",
				"number": "INC0010001",
			},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	p := NewServiceNowProvider(server.URL, "test-token")
	p.client = server.Client()

	result, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey:  "IT-OPS",
		IssueType:   "incident",
		Summary:     "Test ticket",
		Description: "Test description",
		Priority:    "2",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if result.ExternalKey != "INC0010001" {
		t.Errorf("expected key INC0010001, got %s", result.ExternalKey)
	}
}

func TestServiceNowProvider_CreateTicket_DefaultTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/now/table/incident" {
			t.Errorf("expected default table 'incident', got path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]string{"sys_id": "x", "number": "INC0010002"},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	p := NewServiceNowProvider(server.URL, "token")
	p.client = server.Client()

	_, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey: "grp",
		Summary:    "test",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
}

func TestServiceNowProvider_CreateTicket_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		if _, err := w.Write([]byte("forbidden")); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	p := NewServiceNowProvider(server.URL, "bad-token")
	p.client = server.Client()

	_, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey: "grp", Summary: "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
