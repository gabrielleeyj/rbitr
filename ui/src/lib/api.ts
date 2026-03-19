import type { TenantSummary } from "@/lib/tenant";

const defaultBaseUrl = "";
const inflight = new Map<string, Promise<unknown>>();

export interface ApiConfig {
  baseUrl?: string;
  adminKey: string;
}

export interface SetupStatus {
  setup_required: boolean;
  bootstrap_complete: boolean;
  database_reachable: boolean;
  schema_ready: boolean;
  admin_key_count: number;
  tenant_count: number;
  setup_state: "not_started" | "in_progress" | "completed" | "failed";
  last_error?: string;
  initialize_allowed: boolean;
  initialize_token_required: boolean;
  idempotency_required: boolean;
}

export interface SetupInitializeRequest {
  tenant_name: string;
  tenant_id?: string;
  admin_key?: string;
  tenant_key?: string;
}

export interface SetupInitializeOptions {
  setup_token?: string;
  idempotency_key?: string;
}

export interface SetupInitializeResponse {
  bootstrap_complete: boolean;
  tenant_id: string;
  tenant_name: string;
  tenant_key_id: string;
  tenant_key: string;
  tenant_key_created: boolean;
  admin_key_id: string;
  admin_key: string;
  admin_key_created: boolean;
  policy_version: string;
}

export interface AdminMeResponse {
  admin_key_id: string;
  scopes: string[];
}

export interface EvidenceRecord {
  decision_id: string;
  request_id: string;
  tenant_id: string;
  agent_id: string;
  tool_id: string;
  action_type: string;
  action_risk: string;
  decision: string;
  decision_version: string;
  decision_risk: string;
  rule_id: string;
  rule_priority: number;
  reasons: { code: string; message: string }[];
  constraints: Record<string, unknown>;
  tags?: string[];
  policy_version: string;
  reason: string;
  request_hash: string;
  response_hash?: string;
  approval_request_id?: string;
  approval_status?: string;
  approval_decided_at?: string;
  approval_decided_by?: string;
  approval_decision_comment?: string;
  approval_executed_at?: string;
  approval_executed_request_id?: string;
  approval_executed_decision_id?: string;
  approval_request_decision_id?: string;
  timestamp: string;
}

export interface EvidenceResponse {
  tenant_id: string;
  records: EvidenceRecord[];
}

export interface PolicyVersion {
  tenant_id: string;
  policy_version: string;
  rego_module: string;
  created_at: string;
  created_by?: string;
  notes?: string;
}

export interface PolicyVersionsResponse {
  tenant_id: string;
  active_policy_version: string;
  versions: PolicyVersion[];
}

export interface PolicySimulationResponse {
  decision: {
    version: string;
    decision: string;
    risk: string;
    rule: { id: string; priority: number };
    reasons: { code: string; message: string }[];
    constraints: Record<string, unknown>;
    tags?: string[];
    matched_rules?: Array<{
      rule_id: string;
      priority: number;
      effect: "DENY" | "REQUIRE_APPROVAL" | "ALLOW";
      reasons?: { code: string; message: string }[];
      constraints_summary?: Record<string, unknown>;
    }>;
  };
}

export interface HTTPConfig {
  base_url: string;
  auth_type: string;
  auth_set: boolean;
}

export interface MCPConfig {
  upstream_url: string;
  description: string;
  input_schema_json: object;
}

export interface ToolConfig {
  tool_id: string;
  tenant_id: string;
  http?: HTTPConfig;
  mcp?: MCPConfig;
}

export interface TenantKey {
  key_id: string;
  tenant_id: string;
  key_prefix: string;
  created_at: string;
  revoked_at?: string;
  rotated_at?: string;
}

export interface TenantKeyIssueResponse {
  api_key: string;
  key_id: string;
  key_prefix: string;
}

export interface CreateTenantResponse {
  tenant_id: string;
  name: string;
  api_key: string;
  key_id: string;
}

export interface RiskOverride {
  tenant_id: string;
  action_type: string;
  action_risk: string;
  updated_at: string;
}

export interface AdminSettings {
  admin_write_lock: boolean;
  default_approval_ttl_seconds: number;
  audit_retention_days: number;
  disable_x_tenant_key?: boolean;
  feature_rate_limiting?: boolean;
  feature_arg_constraints?: boolean;
  default_rate_limit_per_minute?: number;
  default_rate_limit_per_day?: number;
  default_rate_limit_scope?: "tenant" | "tenant_agent" | "tenant_tool" | "tenant_agent_tool";
  tenant_id?: string;
  enforcement_mode?: "enforce" | "shadow";
  mcp_passthrough_upstream_tool_id?: string;
  feature_session_tokens?: boolean;
  feature_file_governance?: boolean;
  session_token_ttl_seconds?: number;
  secret_provider_aws?: boolean;
  secret_provider_gcp?: boolean;
  secret_provider_vault?: boolean;
  secret_provider_azure?: boolean;
  sso_enabled?: boolean;
  sso_issuer?: string;
  sso_client_id?: string;
  sso_client_secret_ref?: string;
  sso_redirect_uri?: string;
  sso_allowed_domains?: string;
  sso_default_scopes?: string;
}

export interface SSOConfigRequest {
  sso_issuer: string;
  sso_client_id: string;
  sso_client_secret_ref: string;
  sso_redirect_uri: string;
  sso_allowed_domains: string;
  sso_default_scopes: string;
}

export interface SSOStatusResponse {
  sso_enabled: boolean;
}

export interface SSOAuthorizeResponse {
  authorize_url: string;
  state: string;
}

export interface NotificationConfig {
  tenant_id: string;
  slack_webhook_enabled: boolean;
  slack_webhook_configured: boolean;
  slack_webhook_default_channel?: string;
  slack_bot_enabled: boolean;
  slack_bot_configured: boolean;
  slack_bot_default_channel?: string;
  email_enabled: boolean;
  email_configured: boolean;
  email_provider?: string;
  email_from?: string;
  email_region?: string;
  email_domain?: string;
  email_default_mailing_list_id?: string;
  telegram_enabled: boolean;
  telegram_configured: boolean;
  telegram_chat_id?: string;
  whatsapp_enabled: boolean;
  whatsapp_configured: boolean;
  whatsapp_phone_number_id?: string;
  whatsapp_default_recipient?: string;
  notify_approval_expiring: boolean;
  notify_token_abuse: boolean;
  notify_policy_invalid: boolean;
  updated_at?: string;
}

export interface MailingList {
  mailing_list_id: string;
  tenant_id: string;
  name: string;
  description?: string;
  created_at?: string;
  updated_at?: string;
}

export interface AuditEvent {
  audit_event_id: string;
  tenant_id?: string;
  actor_type: string;
  actor_id?: string;
  actor_display?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  request_id?: string;
  ip?: string;
  user_agent?: string;
  created_at: string;
}

export interface AuditResourceTypesResponse {
  resource_types: string[];
}

export interface ApprovalRequest {
  approval_request_id: string;
  tenant_id: string;
  agent_id: string;
  tool_id: string;
  action_type: string;
  request_hash: string;
  request_context?: Record<string, unknown>;
  status: string;
  expires_at: string;
  created_at: string;
  policy_version?: string;
  decided_at?: string;
  decided_by?: string;
  decision_comment?: string;
  executed_at?: string;
  executed_request_id?: string;
  executed_decision_id?: string;
  request_decision_id?: string;
  action_summary?: string;
  risk?: string;
  rule_id?: string;
  reasons?: { code: string; message: string }[];
}

export interface PendingApprovalsCount {
  tenant_id: string;
  pending_count: number;
}

export interface NotificationSuppression {
  dedup_key: string;
  tenant_id: string;
  channel: string;
  event_type: string;
  resource_id?: string;
  severity: string;
  first_seen_at: string;
  last_seen_at: string;
  last_sent_at?: string;
  suppressed_until?: string;
  suppressed_count: number;
  last_payload_hash?: string;
  updated_at: string;
}

export interface NotificationMetadata {
  event_types: string[];
  severities: string[];
  channels: string[];
}

export interface ActionTypesResponse {
  action_types: string[];
}

function apiBaseUrl() {
  return import.meta.env.VITE_API_BASE_URL ?? defaultBaseUrl;
}

async function requestPublic<T>(path: string, init?: RequestInit, baseUrl?: string): Promise<T> {
  const response = await fetch(`${baseUrl ?? apiBaseUrl()}${path}`, {
    ...init,
    cache: init?.cache ?? "no-store",
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed: ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  try {
    return JSON.parse(text) as T;
  } catch {
    throw new Error(invalidJSONError(path, text));
  }
}

async function request<T>(path: string, config: ApiConfig, init?: RequestInit): Promise<T> {
  const method = init?.method?.toUpperCase() ?? "GET";
  const url = `${config.baseUrl ?? apiBaseUrl()}${path}`;
  const inflightKey = `${method}:${url}`;

  if (method === "GET" || method === "HEAD") {
    const existing = inflight.get(inflightKey);
    if (existing) {
      return existing as Promise<T>;
    }
  }

  const requestPromise = (async () => {
    const response = await fetch(url, {
      ...init,
      cache: init?.cache ?? "no-store",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${config.adminKey}`,
        ...init?.headers,
      },
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed: ${response.status}`);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    const text = await response.text();
    if (!text) {
      return undefined as T;
    }
    try {
      return JSON.parse(text) as T;
    } catch {
      throw new Error(invalidJSONError(path, text));
    }
  })();

  if (method === "GET" || method === "HEAD") {
    inflight.set(inflightKey, requestPromise);
    try {
      return await (requestPromise as Promise<T>);
    } finally {
      inflight.delete(inflightKey);
    }
  }

  return requestPromise as Promise<T>;
}

function invalidJSONError(path: string, body: string): string {
  const trimmed = body.trim();
  if (
    trimmed.startsWith("<!doctype html") ||
    trimmed.startsWith("<html") ||
    trimmed.startsWith("<")
  ) {
    return `Expected JSON from ${path} but received HTML. Check Vite proxy or VITE_API_BASE_URL.`;
  }
  return `Expected JSON from ${path} but response was not valid JSON.`;
}

export function getSetupStatus(baseUrl?: string): Promise<SetupStatus> {
  return requestPublic<SetupStatus>("/setup/status", undefined, baseUrl);
}

export function initializeSetup(
  payload: SetupInitializeRequest,
  options?: SetupInitializeOptions,
  baseUrl?: string
): Promise<SetupInitializeResponse> {
  const headers: Record<string, string> = {};
  if (options?.setup_token) {
    headers.Authorization = `Bearer ${options.setup_token}`;
  }
  if (options?.idempotency_key) {
    headers["Idempotency-Key"] = options.idempotency_key;
  }
  return requestPublic<SetupInitializeResponse>(
    "/setup/initialize",
    {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
    },
    baseUrl
  );
}

export function listTenants(config: ApiConfig): Promise<TenantSummary[]> {
  return request<TenantSummary[]>("/admin/tenants", config);
}

export function createTenant(config: ApiConfig, payload: { name: string }): Promise<CreateTenantResponse> {
  return request<CreateTenantResponse>("/admin/tenants", config, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function updateTenantConfig(
  config: ApiConfig,
  tenantId: string,
  payload: { name?: string; tenant_key?: string }
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/config`, config, {
    method: "PUT",
    body: JSON.stringify({
      name: payload.name ?? "",
      tenant_key: payload.tenant_key ?? "",
    }),
  });
}

export function setTenantEnabled(config: ApiConfig, tenantId: string, enabled: boolean): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/enabled`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function deleteTenant(config: ApiConfig, tenantId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}`, config, {
    method: "DELETE",
  });
}

export function getAdminMe(config: ApiConfig): Promise<AdminMeResponse> {
  return request<AdminMeResponse>("/admin/me", config);
}

export async function listTenantKeys(config: ApiConfig, tenantId: string): Promise<TenantKey[]> {
  const data = await request<TenantKey[] | null>(`/admin/tenants/${tenantId}/keys`, config);
  return data ?? [];
}

export function createTenantKey(config: ApiConfig, tenantId: string): Promise<TenantKeyIssueResponse> {
  return request<TenantKeyIssueResponse>(`/admin/tenants/${tenantId}/keys`, config, { method: "POST" });
}

export function rotateTenantKey(config: ApiConfig, tenantId: string): Promise<TenantKeyIssueResponse> {
  return request<TenantKeyIssueResponse>(`/admin/tenants/${tenantId}/keys/rotate`, config, { method: "POST" });
}

export function revokeTenantKey(config: ApiConfig, tenantId: string, keyId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/keys/${keyId}/revoke`, config, { method: "POST" });
}

export function listEvidence(
  config: ApiConfig,
  tenantId: string,
  params: { limit?: number; decision?: string; action_type?: string; risk?: string; since?: string }
): Promise<EvidenceResponse> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.decision) query.set("decision", params.decision);
  if (params.action_type) query.set("action_type", params.action_type);
  if (params.risk) query.set("risk", params.risk);
  if (params.since) query.set("since", params.since);

  return request<EvidenceResponse>(`/admin/tenants/${tenantId}/evidence?${query.toString()}`, config);
}

export function listPolicies(config: ApiConfig, tenantId: string): Promise<PolicyVersionsResponse> {
  return request<PolicyVersionsResponse>(`/admin/tenants/${tenantId}/policies`, config);
}

export function getPolicyVersion(
  config: ApiConfig,
  tenantId: string,
  version: string
): Promise<PolicyVersion> {
  return request<PolicyVersion>(`/admin/tenants/${tenantId}/policies/${version}`, config);
}

export function createPolicyVersion(
  config: ApiConfig,
  tenantId: string,
  payload: { policy_version: string; rego_module: string; notes?: string }
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/policies`, config, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function publishPolicyVersion(
  config: ApiConfig,
  tenantId: string,
  version: string
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/policies/${version}/publish`, config, {
    method: "PUT",
  });
}

export function rollbackPolicyVersion(
  config: ApiConfig,
  tenantId: string,
  version?: string
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/policies/rollback`, config, {
    method: "PUT",
    body: JSON.stringify({ policy_version: version ?? "" }),
  });
}

export function simulatePolicy(
  config: ApiConfig,
  tenantId: string,
  payload: { policy_version?: string; rego_module?: string; input: Record<string, unknown> }
): Promise<PolicySimulationResponse> {
  return request<PolicySimulationResponse>(`/admin/tenants/${tenantId}/policies/simulate`, config, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function listTools(config: ApiConfig, tenantId: string): Promise<ToolConfig[]> {
  return request<ToolConfig[]>(`/admin/tenants/${tenantId}/tools`, config);
}

export function updateTool(
  config: ApiConfig,
  tenantId: string,
  toolId: string,
  payload: { base_url: string; auth_type?: string; auth_value?: string }
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/tools/${toolId}`, config, {
    method: "PUT",
    body: JSON.stringify({
      base_url: payload.base_url,
      auth_type: payload.auth_type ?? "",
      auth_value: payload.auth_value ?? "",
    }),
  });
}

export function updateToolMetadata(
  config: ApiConfig,
  tenantId: string,
  toolId: string,
  payload: {
    description: string;
    mcp_upstream_url: string;
    input_schema_json: object | null;
  }
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/tools/${toolId}/metadata`, config, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export async function listRiskOverrides(config: ApiConfig, tenantId: string): Promise<RiskOverride[]> {
  const data = await request<RiskOverride[] | null>(`/admin/tenants/${tenantId}/risk-overrides`, config);
  return data ?? [];
}

export function upsertRiskOverride(
  config: ApiConfig,
  tenantId: string,
  actionType: string,
  actionRisk: string
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/risk-overrides/${actionType}`, config, {
    method: "PUT",
    body: JSON.stringify({ action_risk: actionRisk }),
  });
}

export function deleteRiskOverride(
  config: ApiConfig,
  tenantId: string,
  actionType: string
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/risk-overrides/${actionType}`, config, {
    method: "DELETE",
  });
}

export async function listApprovals(
  config: ApiConfig,
  tenantId: string,
  params: { status?: string; limit?: number; offset?: number } = {}
): Promise<ApprovalRequest[]> {
  const query = new URLSearchParams();
  if (params.status) query.set("status", params.status);
  if (params.limit) query.set("limit", String(params.limit));
  if (params.offset) query.set("offset", String(params.offset));
  const data = await request<ApprovalRequest[] | null>(
    `/admin/tenants/${tenantId}/approvals?${query.toString()}`,
    config
  );
  return data ?? [];
}

export function getPendingApprovalsCount(
  config: ApiConfig,
  tenantId: string
): Promise<PendingApprovalsCount> {
  return request<PendingApprovalsCount>(
    `/admin/tenants/${tenantId}/approvals/pending-count`,
    config
  );
}

export function getApproval(
  config: ApiConfig,
  tenantId: string,
  approvalRequestId: string
): Promise<ApprovalRequest> {
  return request<ApprovalRequest>(`/admin/tenants/${tenantId}/approvals/${approvalRequestId}`, config);
}

export function approveApproval(
  config: ApiConfig,
  tenantId: string,
  approvalRequestId: string,
  comment?: string
): Promise<ApprovalRequest> {
  return request<ApprovalRequest>(
    `/admin/tenants/${tenantId}/approvals/${approvalRequestId}/approve`,
    config,
    {
      method: "POST",
      body: JSON.stringify({ comment: comment ?? "" }),
    }
  );
}

export function denyApproval(
  config: ApiConfig,
  tenantId: string,
  approvalRequestId: string,
  comment?: string
): Promise<ApprovalRequest> {
  return request<ApprovalRequest>(
    `/admin/tenants/${tenantId}/approvals/${approvalRequestId}/deny`,
    config,
    {
      method: "POST",
      body: JSON.stringify({ comment: comment ?? "" }),
    }
  );
}

export function revokeApproval(
  config: ApiConfig,
  tenantId: string,
  approvalRequestId: string,
  comment?: string
): Promise<ApprovalRequest> {
  return request<ApprovalRequest>(
    `/admin/tenants/${tenantId}/approvals/${approvalRequestId}/revoke`,
    config,
    {
      method: "POST",
      body: JSON.stringify({ comment: comment ?? "" }),
    }
  );
}

export function getSettings(config: ApiConfig, tenantId?: string): Promise<AdminSettings> {
  const query = tenantId ? `?tenant_id=${encodeURIComponent(tenantId)}` : "";
  return request<AdminSettings>(`/admin/settings${query}`, config);
}

export function setAuditRetentionDays(config: ApiConfig, days: number): Promise<void> {
  return request<void>(`/admin/settings/audit-retention`, config, {
    method: "PUT",
    body: JSON.stringify({ days }),
  });
}

export function setAdminWriteLock(config: ApiConfig, locked: boolean): Promise<void> {
  return request<void>(`/admin/settings/admin-write-lock`, config, {
    method: "PUT",
    body: JSON.stringify({ locked }),
  });
}

export function setDefaultApprovalTTL(config: ApiConfig, seconds: number): Promise<void> {
  return request<void>(`/admin/settings/default-approval-ttl`, config, {
    method: "PUT",
    body: JSON.stringify({ seconds }),
  });
}

export function setDefaultRateLimitConfig(
  config: ApiConfig,
  payload: {
    per_minute: number;
    per_day: number;
    scope: "tenant" | "tenant_agent" | "tenant_tool" | "tenant_agent_tool";
  }
): Promise<void> {
  return request<void>(`/admin/settings/default-rate-limit`, config, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export function setDisableXTenantKey(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/disable-x-tenant-key`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setFeatureRateLimiting(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/feature-rate-limiting`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setFeatureArgConstraints(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/feature-arg-constraints`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setFeatureSessionTokens(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/feature-session-tokens`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setFeatureFileGovernance(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/feature-file-governance`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setSessionTokenTTL(config: ApiConfig, seconds: number): Promise<void> {
  return request<void>(`/admin/settings/session-token-ttl`, config, {
    method: "PUT",
    body: JSON.stringify({ ttl_seconds: seconds }),
  });
}

export function setSecretProviderAWS(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/secret-provider-aws`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setSecretProviderGCP(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/secret-provider-gcp`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setSecretProviderVault(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/secret-provider-vault`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setSecretProviderAzure(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/secret-provider-azure`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function setSSOEnabled(config: ApiConfig, enabled: boolean): Promise<void> {
  return request<void>(`/admin/settings/sso-enabled`, config, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function updateSSOConfig(config: ApiConfig, payload: SSOConfigRequest): Promise<void> {
  return request<void>(`/admin/settings/sso-config`, config, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export function getSSOStatus(): Promise<SSOStatusResponse> {
  return requestPublic<SSOStatusResponse>(`/admin/auth/sso/status`);
}

export function getSSOAuthorizeURL(): Promise<SSOAuthorizeResponse> {
  return requestPublic<SSOAuthorizeResponse>(`/admin/auth/sso/authorize`);
}

export interface SSOCallbackResponse {
  token: string;
  expires_at: number;
  email: string;
  name: string;
}

export function ssoCallback(code: string): Promise<SSOCallbackResponse> {
  return requestPublic<SSOCallbackResponse>(`/admin/auth/sso/callback?code=${encodeURIComponent(code)}`);
}

export function ssoLogout(config: ApiConfig): Promise<void> {
  return request<void>(`/admin/auth/sso/logout`, config, {
    method: "POST",
  });
}

export function setEnforcementMode(
  config: ApiConfig,
  tenantId: string,
  enforcementMode: "enforce" | "shadow"
): Promise<void> {
  return request<void>(`/admin/settings/enforcement-mode`, config, {
    method: "PUT",
    body: JSON.stringify({ tenant_id: tenantId, enforcement_mode: enforcementMode }),
  });
}

export function setMCPPassthroughUpstreamTool(
  config: ApiConfig,
  tenantId: string,
  toolId?: string
): Promise<void> {
  return request<void>(`/admin/settings/mcp-passthrough-upstream`, config, {
    method: "PUT",
    body: JSON.stringify({ tenant_id: tenantId, tool_id: toolId ?? "" }),
  });
}

export async function listAuditEvents(
  config: ApiConfig,
  tenantId: string,
  params: {
    limit?: number;
    offset?: number;
    action?: string;
    resource_type?: string;
    actor_id?: string;
    from?: string;
    to?: string;
  } = {}
): Promise<AuditEvent[]> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.offset) query.set("offset", String(params.offset));
  if (params.action) query.set("action", params.action);
  if (params.resource_type) query.set("resource_type", params.resource_type);
  if (params.actor_id) query.set("actor_id", params.actor_id);
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  const data = await request<AuditEvent[] | null>(
    `/admin/tenants/${tenantId}/audit?${query.toString()}`,
    config
  );
  return data ?? [];
}

export function auditExportUrl(
  config: ApiConfig,
  tenantId: string,
  params: {
    format?: "csv" | "json";
    include_details?: boolean;
    action?: string;
    resource_type?: string;
    actor_id?: string;
    from?: string;
    to?: string;
  } = {}
): string {
  const query = new URLSearchParams();
  query.set("format", params.format ?? "csv");
  if (params.include_details) query.set("include_details", "true");
  if (params.action) query.set("action", params.action);
  if (params.resource_type) query.set("resource_type", params.resource_type);
  if (params.actor_id) query.set("actor_id", params.actor_id);
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  return `${config.baseUrl ?? apiBaseUrl()}/admin/tenants/${tenantId}/audit/export?${query.toString()}`;
}

export async function listAuditResourceTypes(config: ApiConfig, tenantId: string): Promise<AuditResourceTypesResponse> {
  return request<AuditResourceTypesResponse>(`/admin/tenants/${tenantId}/audit/resource-types`, config);
}

export async function listAuditEventsAll(
  config: ApiConfig,
  params: { limit?: number; offset?: number; action?: string; resource_type?: string; actor_id?: string } = {}
): Promise<AuditEvent[]> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.offset) query.set("offset", String(params.offset));
  if (params.action) query.set("action", params.action);
  if (params.resource_type) query.set("resource_type", params.resource_type);
  if (params.actor_id) query.set("actor_id", params.actor_id);
  const data = await request<AuditEvent[] | null>(`/admin/audit?${query.toString()}`, config);
  return data ?? [];
}

export function getNotificationConfig(config: ApiConfig, tenantId: string): Promise<NotificationConfig> {
  return request<NotificationConfig>(`/admin/tenants/${tenantId}/notifications`, config);
}

export function getNotificationMetadata(config: ApiConfig): Promise<NotificationMetadata> {
  return request<NotificationMetadata>(`/admin/notifications/event-types`, config);
}

export function getActionTypes(config: ApiConfig): Promise<ActionTypesResponse> {
  return request<ActionTypesResponse>(`/admin/action-types`, config);
}

export function updateNotificationConfig(
  config: ApiConfig,
  tenantId: string,
  payload: Omit<NotificationConfig, "tenant_id" | "slack_webhook_configured" | "slack_bot_configured" | "email_configured" | "telegram_configured" | "whatsapp_configured" | "updated_at">
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications`, config, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export function setSlackSecretRef(config: ApiConfig, tenantId: string, secretRef: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/slack-secret-ref`, config, {
    method: "PUT",
    body: JSON.stringify({ secret_ref: secretRef }),
  });
}

export function setEmailSecretRef(config: ApiConfig, tenantId: string, secretRef: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/email-secret-ref`, config, {
    method: "PUT",
    body: JSON.stringify({ secret_ref: secretRef }),
  });
}

export function sendSlackTest(config: ApiConfig, tenantId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/test/slack`, config, { method: "POST" });
}

export function sendSlackBotTest(config: ApiConfig, tenantId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/test/slack-bot`, config, { method: "POST" });
}

export function sendEmailTest(config: ApiConfig, tenantId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/test/email`, config, { method: "POST" });
}

export function setTelegramSecretRef(config: ApiConfig, tenantId: string, secretRef: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/telegram-secret-ref`, config, {
    method: "PUT",
    body: JSON.stringify({ secret_ref: secretRef }),
  });
}

export function sendTelegramTest(config: ApiConfig, tenantId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/test/telegram`, config, { method: "POST" });
}

export function setWhatsAppSecretRef(config: ApiConfig, tenantId: string, secretRef: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/whatsapp-secret-ref`, config, {
    method: "PUT",
    body: JSON.stringify({ secret_ref: secretRef }),
  });
}

export function sendWhatsAppTest(config: ApiConfig, tenantId: string): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/notifications/test/whatsapp`, config, { method: "POST" });
}

export async function listMailingLists(config: ApiConfig, tenantId: string): Promise<MailingList[]> {
  const data = await request<MailingList[] | null>(`/admin/tenants/${tenantId}/mailing-lists`, config);
  return data ?? [];
}

export function createMailingList(
  config: ApiConfig,
  tenantId: string,
  payload: { name: string; description?: string; members: string[] }
): Promise<MailingList> {
  return request<MailingList>(`/admin/tenants/${tenantId}/mailing-lists`, config, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function updateMailingList(
  config: ApiConfig,
  tenantId: string,
  mailingListId: string,
  payload: { name: string; description?: string; members: string[] }
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/mailing-lists/${mailingListId}`, config, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export function deleteMailingList(
  config: ApiConfig,
  tenantId: string,
  mailingListId: string
): Promise<void> {
  return request<void>(`/admin/tenants/${tenantId}/mailing-lists/${mailingListId}`, config, {
    method: "DELETE",
  });
}

export async function listNotificationSuppressions(
  config: ApiConfig,
  tenantId: string,
  params: { limit?: number; offset?: number; event_type?: string; channel?: string; severity?: string } = {}
): Promise<NotificationSuppression[]> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.offset) query.set("offset", String(params.offset));
  if (params.event_type) query.set("event_type", params.event_type);
  if (params.channel) query.set("channel", params.channel);
  if (params.severity) query.set("severity", params.severity);
  const data = await request<NotificationSuppression[] | null>(
    `/admin/tenants/${tenantId}/notifications/suppressions?${query.toString()}`,
    config
  );
  return data ?? [];
}
