package public

// Field name and value constants used across the public API package.
// Centralized to satisfy goconst and avoid drift across files.
const (
	// JSON token / value strings.
	jsonNull = "null"

	// Common map / response field names.
	fieldMessage           = "message"
	fieldLimit             = "limit"
	fieldAgentID           = "agent_id"
	fieldActionRisk        = "action_risk"
	fieldPolicyVersion     = "policy_version"
	fieldMethod            = "method"
	fieldPath              = "path"
	fieldTenant            = "Tenant"
	fieldTool              = "Tool"
	fieldAction            = "Action"
	fieldReason            = "reason"
	fieldReasons           = "reasons"
	fieldRetryAfterSeconds = "retry_after_seconds"
	fieldExecutionID       = "execution_id"
	fieldApprovalRequestID = "approval_request_id"
	fieldDecision          = "decision"
	fieldRuleID            = "rule_id"
	fieldTenantID          = "tenant_id"
	fieldToolID            = "tool_id"
	fieldWindow            = "window"
	fieldScope             = "scope"
	fieldRisk              = "risk"
	fieldStatus            = "status"
	fieldExpiresAt         = "expires_at"
	fieldBaseURL           = "base_url"
	fieldTransport         = "transport"

	// Decision result strings.
	decisionRequireApproval = "REQUIRE_APPROVAL"
	decisionApproved        = "APPROVED"

	// Risk levels.
	riskMedium = "MEDIUM"

	// MCP method/path constants.
	mcpMethodCall    = "MCP_CALL"
	mcpPathToolsCall = "/tools/call"

	// Auth type values.
	authTypeAPIKey = "api_key"

	// Content type / body type constants.
	mimeApplicationJSON = "application/json"
	contentTypeText     = "text"

	// Error / status codes & reason strings.
	codeRateLimitExceeded         = "RATE_LIMIT_EXCEEDED"
	errFailedPersistDecision      = "failed to persist decision"
	errFilePathTraversal          = "file path traversal detected"
	errFileOutsideSandbox         = "file access outside tenant sandbox"
	errApprovalAlreadyClaimed     = "approval already claimed"
	errToolNotFoundMessage        = "tool not found"
	errTenantMismatch             = "tenant mismatch"
	errUnauthorized               = "unauthorized"
	errUpstreamToolExecFailed     = "upstream tool execution failed"
	errAuthenticationFailed       = "authentication failed"
	reasonApprovalAlreadyExecuted = "approval_already_executed"
	reasonAlreadyClaimed          = "already_claimed"

	// HTTP headers.
	headerContentType = "Content-Type"
)
