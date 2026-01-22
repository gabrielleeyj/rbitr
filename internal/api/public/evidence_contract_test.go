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
	DecisionID                 string           `json:"decision_id"`
	RequestID                  string           `json:"request_id"`
	TenantID                   string           `json:"tenant_id" validate:"required"`
	AgentID                    string           `json:"agent_id" validate:"required"`
	ToolID                     string           `json:"tool_id" validate:"required"`
	ActionType                 string           `json:"action_type" validate:"required"`
	ActionRisk                 string           `json:"action_risk"`
	ActionSummary              string           `json:"action_summary" validate:"max=500"`
	Decision                   string           `json:"decision" validate:"required,oneof=ALLOW DENY REQUIRE_APPROVAL"`
	DecisionVersion            string           `json:"decision_version" validate:"required"`
	DecisionRisk               string           `json:"decision_risk" validate:"required,oneof=LOW MEDIUM HIGH CRITICAL"`
	RuleID                     string           `json:"rule_id" validate:"required"`
	RulePriority               int              `json:"rule_priority" validate:"required"`
	Reasons                    []reasonContract `json:"reasons" validate:"required,min=1,dive"`
	Constraints                map[string]any   `json:"constraints" validate:"required"`
	Tags                       []string         `json:"tags"`
	PolicyVersion              string           `json:"policy_version" validate:"required"`
	Reason                     string           `json:"reason" validate:"max=500"`
	RequestHash                string           `json:"request_hash" validate:"required,sha256hash"`
	ResponseHash               string           `json:"response_hash" validate:"omitempty,sha256hash"`
	ApprovalRequestID          string           `json:"approval_request_id"`
	ApprovalStatus             string           `json:"approval_status"`
	ApprovalDecidedAt          *time.Time       `json:"approval_decided_at"`
	ApprovalDecidedBy          string           `json:"approval_decided_by"`
	ApprovalComment            string           `json:"approval_decision_comment"`
	ApprovalExecutedAt         *time.Time       `json:"approval_executed_at"`
	ApprovalExecutedRequestID  string           `json:"approval_executed_request_id"`
	ApprovalExecutedDecisionID string           `json:"approval_executed_decision_id"`
	ApprovalRequestDecisionID  string           `json:"approval_request_decision_id"`
	Timestamp                  time.Time        `json:"timestamp" validate:"required"`
}

type reasonContract struct {
	Code    string `json:"code" validate:"required"`
	Message string `json:"message" validate:"required"`
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
