package utils

import "testing"

func TestFilterHeaders(t *testing.T) {
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "secret",
		"Accept":        "*/*",
		"Cookie":        "bad",
	}

	filtered := FilterHeaders(headers)
	if _, ok := filtered["content-type"]; !ok {
		t.Fatalf("expected content-type to be allowed")
	}
	if _, ok := filtered["accept"]; !ok {
		t.Fatalf("expected accept to be allowed")
	}
	if _, ok := filtered["authorization"]; ok {
		t.Fatalf("expected authorization to be filtered")
	}
	if _, ok := filtered["cookie"]; ok {
		t.Fatalf("expected cookie to be filtered")
	}
}

func TestHashCanonicalStable(t *testing.T) {
	canonical := CanonicalRequest{
		TenantID:       "t1",
		AgentID:        "a1",
		ToolID:         "tool",
		Method:         "POST",
		Path:           "/refund",
		Query:          "",
		Headers:        map[string]string{"content-type": "application/json", "accept": "*/*"},
		BodyHash:       "sha256:abc",
		IdempotencyKey: "idem",
	}
	first := HashCanonical(&canonical)
	second := HashCanonical(&canonical)
	if first != second {
		t.Fatalf("expected stable hash, got %s and %s", first, second)
	}
}
