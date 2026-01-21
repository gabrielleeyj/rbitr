package models

import (
	"encoding/json"
	"time"
)

type Tenant struct {
	TenantID string
	Name     string
}

type TenantSummary struct {
	TenantID            string `json:"tenant_id"`
	Name                string `json:"name"`
	ActivePolicyVersion string `json:"active_policy_version"`
	ToolCount           int    `json:"tool_count"`
}

type Tool struct {
	ToolID    string `json:"tool_id"`
	TenantID  string `json:"tenant_id"`
	BaseURL   string `json:"base_url"`
	AuthType  string `json:"auth_type"`
	AuthValue string `json:"auth_value"`
}

type RiskOverride struct {
	TenantID   string    `json:"tenant_id"`
	ActionType string    `json:"action_type"`
	ActionRisk string    `json:"action_risk"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Policy struct {
	PolicyID      string
	TenantID      string
	RegoModule    string
	PolicyVersion string
	UpdatedAt     time.Time
}

type PolicyVersion struct {
	TenantID      string    `json:"tenant_id"`
	PolicyVersion string    `json:"policy_version"`
	RegoModule    string    `json:"rego_module"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
	Notes         string    `json:"notes"`
}

type TenantConfig struct {
	TenantID            string    `json:"tenant_id"`
	ActivePolicyVersion string    `json:"active_policy_version"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type AdminAuditEvent struct {
	AuditEventID string          `json:"audit_event_id"`
	TenantID     string          `json:"tenant_id"`
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id"`
	ActorDisplay string          `json:"actor_display"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Before       json.RawMessage `json:"before"`
	After        json.RawMessage `json:"after"`
	RequestID    string          `json:"request_id"`
	IP           string          `json:"ip"`
	UserAgent    string          `json:"user_agent"`
	CreatedAt    time.Time       `json:"created_at"`
}

type ActionDecisionRecord struct {
	DecisionID        string           `json:"decision_id,omitempty"`
	RequestID         string           `json:"request_id,omitempty"`
	TenantID          string           `json:"tenant_id,omitempty"`
	AgentID           string           `json:"agent_id,omitempty"`
	ToolID            string           `json:"tool_id,omitempty"`
	ActionType        string           `json:"action_type,omitempty"`
	ActionRisk        string           `json:"action_risk,omitempty"`
	ActionSummary     string           `json:"action_summary,omitempty"`
	Decision          string           `json:"decision,omitempty"`
	DecisionVersion   string           `json:"decision_version,omitempty"`
	DecisionRisk      string           `json:"decision_risk,omitempty"`
	RuleID            string           `json:"rule_id,omitempty"`
	RulePriority      int              `json:"rule_priority,omitempty"`
	Reasons           []DecisionReason `json:"reasons,omitempty"`
	Constraints       map[string]any   `json:"constraints,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	PolicyVersion     string           `json:"policy_version,omitempty"`
	Reason            string           `json:"reason,omitempty"`
	RequestHash       string           `json:"request_hash,omitempty"`
	ResponseHash      string           `json:"response_hash,omitempty"`
	ApprovalRequestID string           `json:"approval_request_id,omitempty"`
	CreatedAt         time.Time        `json:"created_at,omitempty"`
}

type DecisionRule struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
}

type DecisionReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ActionDecisionExport struct {
	DecisionID        string           `json:"decision_id"`
	RequestID         string           `json:"request_id"`
	TenantID          string           `json:"tenant_id"`
	AgentID           string           `json:"agent_id"`
	ToolID            string           `json:"tool_id"`
	ActionType        string           `json:"action_type"`
	ActionRisk        string           `json:"action_risk"`
	ActionSummary     string           `json:"action_summary"`
	Decision          string           `json:"decision"`
	DecisionVersion   string           `json:"decision_version"`
	DecisionRisk      string           `json:"decision_risk"`
	RuleID            string           `json:"rule_id"`
	RulePriority      int              `json:"rule_priority"`
	Reasons           []DecisionReason `json:"reasons"`
	Constraints       map[string]any   `json:"constraints"`
	Tags              []string         `json:"tags,omitempty"`
	PolicyVersion     string           `json:"policy_version"`
	Reason            string           `json:"reason"`
	RequestHash       string           `json:"request_hash"`
	ResponseHash      string           `json:"response_hash,omitempty"`
	ApprovalRequestID string           `json:"approval_request_id,omitempty"`
	Timestamp         time.Time        `json:"timestamp"`
}

type ApprovalRequest struct {
	ApprovalRequestID string
	TenantID          string
	AgentID           string
	ToolID            string
	ActionType        string
	RequestHash       string
	Status            string
	ExpiresAt         time.Time
	CreatedAt         time.Time
}

type TenantKey struct {
	TenantID string
	KeyHash  string
}

type AdminKey struct {
	AdminKeyID string
	KeyHash    string
	Scopes     []string
}
