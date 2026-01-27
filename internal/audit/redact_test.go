package audit

import "testing"

func TestRedactPayloadAllowlist(t *testing.T) {
	payload := map[string]any{
		"base_url":   "http://example",
		"auth_type":  "token",
		"auth_value": "secret",
		"auth_set":   true,
	}
	redacted := RedactPayload("TOOL", payload)
	if _, ok := redacted["auth_value"]; ok {
		t.Fatalf("expected auth_value to be redacted")
	}
	if redacted["base_url"] != "http://example" {
		t.Fatalf("expected base_url to be retained")
	}
	if redacted["auth_set"] != true {
		t.Fatalf("expected auth_set to be retained")
	}
}
