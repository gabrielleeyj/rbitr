package models

import "time"

type Tenant struct {
	TenantID string
	Name     string
}

type Tool struct {
	ToolID   string
	TenantID string
	BaseURL  string
	AuthType string
	AuthValue string
}

type Policy struct {
	PolicyID     string
	TenantID     string
	RegoModule   string
	PolicyVersion string
	UpdatedAt    time.Time
}

type ActionDecisionRecord struct {
	DecisionID       string
	RequestID        string
	TenantID         string
	AgentID          string
	ToolID           string
	ActionType       string
	ActionRisk       string
	ActionSummary    string
	Decision         string
	Reason           string
	RuleID           string
	PolicyVersion    string
	RequestHash      string
	ResponseHash     string
	ApprovalRequestID string
	CreatedAt        time.Time
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
