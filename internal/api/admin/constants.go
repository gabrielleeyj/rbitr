package admin

const (
	fieldDetail             = "detail"
	fieldDecision           = "decision"
	fieldConfigured         = "configured"
	fieldTenant             = "Tenant"
	fieldEmail              = "email"
	fieldBaseURL            = "base_url"
	fieldAuthSet            = "auth_set"
	fieldCredentialProvider = "credential_provider" //nolint:gosec // field name, not a credential
	fieldMessage            = "message"
	fieldName               = "name"
	fieldFeatures           = "features"
	fieldTier               = "tier"

	severityInfo = "INFO"
	severityHigh = "HIGH"

	actorTypeAdminKey = "admin_key"

	featureApprovalWorkflows = "approval_workflows"
	featureCustomPolicies    = "custom_policies"
	featureEvidenceExport    = "evidence_export"
	featureIntegrations      = "integrations"

	eventNotificationsTest = "NOTIFICATIONS.TEST"
	testNotificationBody   = "This is a test notification from rbitr."

	errSSONotConfigured       = "SSO not configured"
	errInvalidRequestBody     = "invalid request body"
	errInvalidLimit           = "invalid limit"
	errInvalidOffset          = "invalid offset"
	errInvalidFrom            = "invalid from"
	errInvalidTo              = "invalid to"
	errInvalidSecretRef       = "invalid secret_ref"
	errFailedLoadApproval     = "failed to load approval"
	errFailedLoadTenantConfig = "failed to load tenant config"
	errFailedLoadNotifConfig  = "failed to load notification config"
	errNotifConfigMissing     = "notification config missing"

	tierFreeStr = "free"

	transportMCP = "mcp"
	authBearer   = "bearer"

	fieldTenantID      = "tenant_id"
	fieldPolicyVersion = "policy_version"
	fieldValue         = "value"
	fieldScope         = "scope"
	fieldStatus        = "status"
	fieldNotes         = "notes"
	fieldRegoSHA256    = "rego_sha256"

	rateLimitScopeTenantAgentTool = "tenant_agent_tool"
	rateLimitScopeTenantAgent     = "tenant_agent"
	rateLimitScopeTenantTool      = "tenant_tool"

	resourceTest = "test"

	errTenantNotFound        = "tenant not found"
	errPolicyVersionNotFound = "policy version not found"
	errTenantConfigNotFound  = "tenant config not found"
	errNotifNotConfigured    = "notifications not configured"
	errToolNotFound          = "tool not found"
	errUnauthorized          = "unauthorized"
	errForbidden             = "forbidden"

	fieldValid = "valid"

	scopeAdminRead  = "admin:read"
	scopeAdminWrite = "admin:write"

	errFailedGenerateKey = "failed to generate key"

	fieldKeyPrefix         = "key_prefix"
	fieldEnabled           = "enabled"
	fieldProvider          = "provider"
	fieldApprovalRequestID = "approval_request_id"
	fieldToolID            = "tool_id"
)
