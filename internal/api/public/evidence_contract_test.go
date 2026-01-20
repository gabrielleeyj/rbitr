package public

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

type evidenceExportContract struct {
	TenantID string                   `json:"tenant_id" validate:"required"`
	Records  []evidenceRecordContract `json:"records" validate:"max=50,dive"`
}

type evidenceRecordContract struct {
	DecisionID        string    `json:"decision_id"`
	RequestID         string    `json:"request_id"`
	TenantID          string    `json:"tenant_id" validate:"required"`
	AgentID           string    `json:"agent_id" validate:"required"`
	ToolID            string    `json:"tool_id" validate:"required"`
	ActionType        string    `json:"action_type" validate:"required"`
	ActionRisk        string    `json:"action_risk"`
	ActionSummary     string    `json:"action_summary" validate:"max=500"`
	Decision          string    `json:"decision" validate:"required,oneof=ALLOW DENY REQUIRE_APPROVAL"`
	Reason            string    `json:"reason" validate:"max=500"`
	RuleID            string    `json:"rule_id"`
	PolicyVersion     string    `json:"policy_version"`
	RequestHash       string    `json:"request_hash" validate:"required,sha256hash"`
	ResponseHash      string    `json:"response_hash" validate:"omitempty,sha256hash"`
	ApprovalRequestID string    `json:"approval_request_id"`
	Timestamp         time.Time `json:"timestamp" validate:"required"`
}

func validateEvidenceContract(t *testing.T, body []byte) {
	var payload evidenceExportContract
	require.NoError(t, json.Unmarshal(body, &payload))

	validate := validator.New()
	require.NoError(t, validate.RegisterValidation("sha256hash", validateSHA256Hash))
	require.NoError(t, validate.Struct(payload))
}

func validateSHA256Hash(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	hash := strings.TrimPrefix(value, "sha256:")
	if len(hash) != 64 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}
