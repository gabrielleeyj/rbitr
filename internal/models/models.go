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
	TenantID            string
	Name                string
	ActivePolicyVersion string
	ToolCount           int
}

type Tool struct {
	ToolID    string
	TenantID  string
	BaseURL   string
	AuthType  string
	AuthValue string
}

type RiskOverride struct {
	TenantID   string
	ActionType string
	ActionRisk string
	UpdatedAt  time.Time
}

type Policy struct {
	PolicyID      string
	TenantID      string
	RegoModule    string
	PolicyVersion string
	UpdatedAt     time.Time
}

type PolicyVersion struct {
	TenantID      string
	PolicyVersion string
	RegoModule    string
	CreatedAt     time.Time
	CreatedBy     string
	Notes         string
}

type TenantConfig struct {
	TenantID            string
	ActivePolicyVersion string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type AdminAuditEvent struct {
	AuditEventID string
	TenantID     string
	ActorType    string
	ActorID      string
	ActorDisplay string
	Action       string
	ResourceType string
	ResourceID   string
	Before       json.RawMessage
	After        json.RawMessage
	RequestID    string
	IP           string
	UserAgent    string
	CreatedAt    time.Time
}

type ActionDecisionRecord struct {
	DecisionID        string
	RequestID         string
	TenantID          string
	AgentID           string
	ToolID            string
	ActionType        string
	ActionRisk        string
	ActionSummary     string
	Decision          string
	DecisionVersion   string
	DecisionRisk      string
	RuleID            string
	RulePriority      int
	Reasons           []DecisionReason
	Constraints       map[string]any
	Tags              []string
	PolicyVersion     string
	Reason            string
	RequestHash       string
	ResponseHash      string
	ApprovalRequestID string
	CreatedAt         time.Time
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
