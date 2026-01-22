import type { TenantSummary } from "@/lib/tenant";

const defaultBaseUrl = "";
const inflight = new Map<string, Promise<unknown>>();

export interface ApiConfig {
  baseUrl?: string;
  adminKey: string;
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
  };
}

export interface ToolConfig {
  tool_id: string;
  tenant_id: string;
  base_url: string;
  auth_type: string;
  auth_set: boolean;
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

export interface ApprovalRequest {
  approval_request_id: string;
  tenant_id: string;
  agent_id: string;
  tool_id: string;
  action_type: string;
  request_hash: string;
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

function apiBaseUrl() {
  return import.meta.env.VITE_API_BASE_URL ?? defaultBaseUrl;
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
    return JSON.parse(text) as T;
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

export function listTenants(config: ApiConfig): Promise<TenantSummary[]> {
  return request<TenantSummary[]>("/admin/tenants", config);
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

export function getSettings(config: ApiConfig): Promise<AdminSettings> {
  return request<AdminSettings>(`/admin/settings`, config);
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

export async function listAuditEvents(
  config: ApiConfig,
  tenantId: string,
  params: { limit?: number; offset?: number; action?: string; resource_type?: string; actor_id?: string } = {}
): Promise<AuditEvent[]> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.offset) query.set("offset", String(params.offset));
  if (params.action) query.set("action", params.action);
  if (params.resource_type) query.set("resource_type", params.resource_type);
  if (params.actor_id) query.set("actor_id", params.actor_id);
  const data = await request<AuditEvent[] | null>(
    `/admin/tenants/${tenantId}/audit?${query.toString()}`,
    config
  );
  return data ?? [];
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
