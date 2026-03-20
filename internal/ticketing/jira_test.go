package ticketing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJiraProvider_CreateTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content type")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		fields, ok := body["fields"].(map[string]any)
		if !ok {
			t.Fatal("missing fields")
		}
		project, _ := fields["project"].(map[string]any)
		if project["key"] != "PROJ" {
			t.Errorf("expected project key PROJ, got %v", project["key"])
		}

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]string{"key": "PROJ-123", "self": "/rest/api/3/issue/123"}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	p := NewJiraProvider(server.URL, "test-token", "")
	p.client = server.Client()

	result, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey:  "PROJ",
		IssueType:   "Task",
		Summary:     "Test ticket",
		Description: "Test description",
		Priority:    "High",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if result.ExternalKey != "PROJ-123" {
		t.Errorf("expected key PROJ-123, got %s", result.ExternalKey)
	}
	expectedURL := server.URL + "/browse/PROJ-123"
	if result.ExternalURL != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, result.ExternalURL)
	}
}

func TestJiraProvider_CreateTicket_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"errorMessages":["bad request"]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	p := NewJiraProvider(server.URL, "test-token", "")
	p.client = server.Client()

	_, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey: "PROJ",
		IssueType:  "Task",
		Summary:    "Test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJiraProvider_UpdateTicket_Comment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/PROJ-1/comment" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewJiraProvider(server.URL, "test-token", "")
	p.client = server.Client()

	err := p.UpdateTicket(context.Background(), &UpdateTicketRequest{
		ExternalKey: "PROJ-1",
		Comment:     "Approved",
	})
	if err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}
}

func TestJiraProvider_BasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user@test.com" || pass != "api-token" {
			t.Errorf("expected basic auth with user@test.com:api-token, got %s:%s ok=%v", user, pass, ok)
		}
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]string{"key": "T-1"}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	p := NewJiraProvider(server.URL, "api-token", "user@test.com")
	p.client = server.Client()

	_, err := p.CreateTicket(context.Background(), &CreateTicketRequest{
		ProjectKey: "T", IssueType: "Task", Summary: "test",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
}
