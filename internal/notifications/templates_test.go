package notifications

import "testing"

func TestBuildMessageUsesDefaults(t *testing.T) {
	msg := BuildMessage("UNKNOWN.EVENT", map[string]string{"reason": "oops"})
	if msg.Title != "UNKNOWN.EVENT" {
		t.Fatalf("expected title fallback to event type, got %q", msg.Title)
	}
	if msg.Body != "oops" {
		t.Fatalf("expected body to use reason, got %q", msg.Body)
	}
}

func TestBuildMessageUsesSummary(t *testing.T) {
	msg := BuildMessage(EventApprovalExpiring, map[string]string{
		"summary": "Approval expiring",
		"Tenant":  "t1",
	})
	if msg.Title == "" {
		t.Fatalf("expected title")
	}
	if msg.Body != "Approval expiring" {
		t.Fatalf("expected summary body, got %q", msg.Body)
	}
	if msg.Fields["Tenant"] != "t1" {
		t.Fatalf("expected tenant field")
	}
}
