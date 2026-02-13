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
	ToolID          string          `json:"tool_id"`
	TenantID        string          `json:"tenant_id"`
	BaseURL         string          `json:"base_url"`
	AuthType        string          `json:"auth_type"`
	AuthValue       string          `json:"auth_value"`
	Transport       string          `json:"transport"`
	MCPUpstreamURL  string          `json:"mcp_upstream_url,omitempty"`
	Description     string          `json:"description,omitempty"`
	InputSchemaJSON json.RawMessage `json:"input_schema_json,omitempty"`
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
	StreamID     string          `json:"stream_id,omitempty"`
	EventHash    string          `json:"event_hash,omitempty"`
	PrevHash     string          `json:"prev_hash,omitempty"`
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id"`
	ActorDisplay string          `json:"actor_display"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	RequestID    string          `json:"request_id,omitempty"`
	IP           string          `json:"ip,omitempty"`
	UserAgent    string          `json:"user_agent,omitempty"`
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
	CreatedAt         time.Time        `json:"created_at"`
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
	DecisionID                 string           `json:"decision_id"`
	RequestID                  string           `json:"request_id"`
	TenantID                   string           `json:"tenant_id"`
	AgentID                    string           `json:"agent_id"`
	ToolID                     string           `json:"tool_id"`
	ActionType                 string           `json:"action_type"`
	ActionRisk                 string           `json:"action_risk"`
	ActionSummary              string           `json:"action_summary"`
	Decision                   string           `json:"decision"`
	DecisionVersion            string           `json:"decision_version"`
	DecisionRisk               string           `json:"decision_risk"`
	RuleID                     string           `json:"rule_id"`
	RulePriority               int              `json:"rule_priority"`
	Reasons                    []DecisionReason `json:"reasons"`
	Constraints                map[string]any   `json:"constraints"`
	Tags                       []string         `json:"tags,omitempty"`
	PolicyVersion              string           `json:"policy_version"`
	Reason                     string           `json:"reason"`
	RequestHash                string           `json:"request_hash"`
	ResponseHash               string           `json:"response_hash,omitempty"`
	ApprovalRequestID          string           `json:"approval_request_id,omitempty"`
	ApprovalStatus             string           `json:"approval_status,omitempty"`
	ApprovalDecidedAt          *time.Time       `json:"approval_decided_at,omitempty"`
	ApprovalDecidedBy          string           `json:"approval_decided_by,omitempty"`
	ApprovalComment            string           `json:"approval_decision_comment,omitempty"`
	ApprovalExecutedAt         *time.Time       `json:"approval_executed_at,omitempty"`
	ApprovalExecutedRequestID  string           `json:"approval_executed_request_id,omitempty"`
	ApprovalExecutedDecisionID string           `json:"approval_executed_decision_id,omitempty"`
	ApprovalRequestDecisionID  string           `json:"approval_request_decision_id,omitempty"`
	Timestamp                  time.Time        `json:"timestamp"`
}

type ApprovalRequest struct {
	ApprovalRequestID  string           `json:"approval_request_id"`
	TenantID           string           `json:"tenant_id"`
	AgentID            string           `json:"agent_id"`
	ToolID             string           `json:"tool_id"`
	ActionType         string           `json:"action_type"`
	RequestHash        string           `json:"request_hash"`
	Status             string           `json:"status"`
	ApprovalTokenHash  string           `json:"-"`
	ExpiresAt          time.Time        `json:"expires_at"`
	CreatedAt          time.Time        `json:"created_at"`
	PolicyVersion      string           `json:"policy_version,omitempty"`
	DecidedAt          *time.Time       `json:"decided_at,omitempty"`
	DecidedBy          string           `json:"decided_by,omitempty"`
	DecisionComment    string           `json:"decision_comment,omitempty"`
	ExecutedAt         *time.Time       `json:"executed_at,omitempty"`
	ExecutedRequestID  string           `json:"executed_request_id,omitempty"`
	ExecutedDecisionID string           `json:"executed_decision_id,omitempty"`
	RequestDecisionID  string           `json:"request_decision_id,omitempty"`
	ActionSummary      string           `json:"action_summary,omitempty"`
	Risk               string           `json:"risk,omitempty"`
	RuleID             string           `json:"rule_id,omitempty"`
	Reasons            []DecisionReason `json:"reasons,omitempty"`
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

type NotificationConfig struct {
	TenantID                   string    `json:"tenant_id"`
	SlackWebhookEnabled        bool      `json:"slack_webhook_enabled"`
	SlackWebhookSecretRef      string    `json:"-"`
	SlackWebhookDefaultChannel string    `json:"slack_webhook_default_channel"`
	SlackBotEnabled            bool      `json:"slack_bot_enabled"`
	SlackBotSecretRef          string    `json:"-"`
	SlackBotDefaultChannel     string    `json:"slack_bot_default_channel"`
	SlackBotSigningSecretRef   string    `json:"-"`
	EmailEnabled               bool      `json:"email_enabled"`
	EmailProvider              string    `json:"email_provider"`
	EmailSecretRef             string    `json:"-"`
	EmailFrom                  string    `json:"email_from"`
	EmailRegion                string    `json:"email_region"`
	EmailDomain                string    `json:"email_domain"`
	EmailDefaultMailingListID  string    `json:"email_default_mailing_list_id"`
	NotifyApprovalExpiring     bool      `json:"notify_approval_expiring"`
	NotifyTokenAbuse           bool      `json:"notify_token_abuse"`
	NotifyPolicyInvalid        bool      `json:"notify_policy_invalid"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type MailingList struct {
	MailingListID string    `json:"mailing_list_id"`
	TenantID      string    `json:"tenant_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MailingListMember struct {
	MailingListID string    `json:"mailing_list_id"`
	Email         string    `json:"email"`
	CreatedAt     time.Time `json:"created_at"`
}

type NotificationSuppression struct {
	DedupKey        string     `json:"dedup_key"`
	TenantID        string     `json:"tenant_id"`
	Channel         string     `json:"channel"`
	EventType       string     `json:"event_type"`
	ResourceID      string     `json:"resource_id"`
	Severity        string     `json:"severity"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	LastSentAt      *time.Time `json:"last_sent_at"`
	SuppressedUntil *time.Time `json:"suppressed_until"`
	SuppressedCount int64      `json:"suppressed_count"`
	LastPayloadHash string     `json:"last_payload_hash"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
