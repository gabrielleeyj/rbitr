package audit

import (
	"encoding/json"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

const HashSchemaVersion = "audit_v1"

type HashPayload struct {
	SchemaVersion string          `json:"schema_version"`
	StreamID      string          `json:"stream_id"`
	AuditEventID  string          `json:"audit_event_id"`
	CreatedAt     string          `json:"created_at"`
	ActorType     string          `json:"actor_type"`
	ActorID       string          `json:"actor_id"`
	Action        string          `json:"action"`
	ResourceType  string          `json:"resource_type"`
	ResourceID    string          `json:"resource_id"`
	RequestID     string          `json:"request_id"`
	Before        json.RawMessage `json:"before_redacted"`
	After         json.RawMessage `json:"after_redacted"`
}

func BuildHashPayload(event models.AdminAuditEvent, streamID string) HashPayload {
	createdAt := event.CreatedAt.UTC()
	return HashPayload{
		SchemaVersion: HashSchemaVersion,
		StreamID:      streamID,
		AuditEventID:  event.AuditEventID,
		CreatedAt:     createdAt.Format(time.RFC3339Nano),
		ActorType:     event.ActorType,
		ActorID:       event.ActorID,
		Action:        event.Action,
		ResourceType:  event.ResourceType,
		ResourceID:    event.ResourceID,
		RequestID:     event.RequestID,
		Before:        event.Before,
		After:         event.After,
	}
}

func ComputeEventHash(prevHash string, payload HashPayload) (string, error) {
	canonical, err := CanonicalJSON(payload)
	if err != nil {
		return "", err
	}
	return utils.HashString(prevHash + string(canonical)), nil
}
