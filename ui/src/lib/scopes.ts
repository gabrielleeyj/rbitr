export const scopeTenantsRead = "admin:tenants:read";
export const scopeTenantsWrite = "admin:tenants:write";
export const scopeKeysRead = "admin:keys:read";
export const scopeKeysRotate = "admin:keys:rotate";
export const scopeKeysRevoke = "admin:keys:revoke";
export const scopePoliciesRead = "admin:policies:read";
export const scopePoliciesWrite = "admin:policies:write";
export const scopePoliciesPublish = "admin:policies:publish";
export const scopePoliciesRollback = "admin:policies:rollback";
export const scopePoliciesSimulate = "admin:policies:simulate";
export const scopeToolsRead = "admin:tools:read";
export const scopeToolsWrite = "admin:tools:write";
export const scopeApprovalsRead = "admin:approvals:read";
export const scopeApprovalsDecide = "admin:approvals:decide";
export const scopeAuditRead = "admin:audit:read";
export const scopeAuditExport = "admin:audit:export";
export const scopeNotificationsRead = "admin:notifications:read";
export const scopeNotificationsWrite = "admin:notifications:write";
export const scopeNotificationsTest = "admin:notifications:test";
export const scopeTicketingRead = "admin:ticketing:read";
export const scopeTicketingWrite = "admin:ticketing:write";
export const scopeTicketingTest = "admin:ticketing:test";
export const scopeSettingsRead = "admin:settings:read";
export const scopeSettingsWrite = "admin:settings:write";
export const scopeLicenseRead = "admin:settings:read";
export const scopeLicenseWrite = "admin:settings:write";

function isReadLikeScope(required: string): boolean {
  if (required === "admin:read") {
    return true;
  }
  return (
    required.endsWith(":read") ||
    required.endsWith(":export") ||
    required.endsWith(":simulate")
  );
}

export function hasAdminScope(scopes: string[], required: string): boolean {
  if (scopes.includes(required)) {
    return true;
  }
  if (isReadLikeScope(required)) {
    return scopes.includes("admin:read");
  }
  return scopes.includes("admin:write");
}
