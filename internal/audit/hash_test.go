package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func TestComputeEventHashDeterministic(t *testing.T) {
	event := models.AdminAuditEvent{
		AuditEventID: "ae_1",
		TenantID:     "t1",
		ActorType:    "admin_key",
		ActorID:      "admin",
		Action:       "POLICY.VERSION.PUBLISH",
		ResourceType: "POLICY.ACTIVE",
		ResourceID:   "p_v1",
		RequestID:    "req",
		CreatedAt:    time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC),
	}
	before := map[string]any{"active_policy_version": "p_v0"}
	after := map[string]any{"active_policy_version": "p_v1"}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	event.Before = beforeJSON
	event.After = afterJSON

	payload := BuildHashPayload(&event, "t1")
	hash1, err := ComputeEventHash("prev", &payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2, err := ComputeEventHash("prev", &payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash1 != hash2 {
		t.Fatalf("expected deterministic hash")
	}
}
