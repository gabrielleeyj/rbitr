import type { TenantSummary } from "@/lib/tenant";

const defaultBaseUrl = "";

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
  timestamp: string;
}

export interface EvidenceResponse {
  tenant_id: string;
  records: EvidenceRecord[];
}

function apiBaseUrl() {
  return import.meta.env.VITE_API_BASE_URL ?? defaultBaseUrl;
}

async function request<T>(path: string, config: ApiConfig, init?: RequestInit): Promise<T> {
  const response = await fetch(`${config.baseUrl ?? apiBaseUrl()}${path}`, {
    ...init,
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

  return (await response.json()) as T;
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
