package ticketing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLinearProvider_CreateTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-token" {
			t.Errorf("missing auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content type")
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issueCreate": map[string]any{
					"success": true,
					"issue": map[string]any{
						"identifier": "ENG-42",
						"url":        "https://linear.app/team/issue/ENG-42",
					},
				},
			},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	p := NewLinearProvider("test-token")
	p.client = server.Client()
	p.apiURL = server.URL

	result, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey:  "team-id",
		Summary:     "Test ticket",
		Description: "Test description",
		Priority:    "High",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if result.ExternalKey != "ENG-42" {
		t.Errorf("expected key ENG-42, got %s", result.ExternalKey)
	}
	if result.ExternalURL != "https://linear.app/team/issue/ENG-42" {
		t.Errorf("unexpected URL: %s", result.ExternalURL)
	}
}

func TestLinearProvider_CreateTicket_GraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{
				{"message": "Team not found"},
			},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	p := NewLinearProvider("test-token")
	p.client = server.Client()
	p.apiURL = server.URL

	_, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey: "bad-team",
		Summary:    "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLinearPriorityNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"Urgent", 1},
		{"High", 2},
		{"Medium", 3},
		{"Low", 4},
		{"Unknown", 0},
	}
	for _, tt := range tests {
		got := linearPriorityNumber(tt.input)
		if got != tt.expected {
			t.Errorf("linearPriorityNumber(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
