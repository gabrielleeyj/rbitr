import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  createTenant,
  createTenantKey,
  deleteTenant,
  getNotificationConfig,
  getPendingApprovalsCount,
  listTenantKeys,
  listTenants,
  revokeTenantKey,
  rotateTenantKey,
  setTenantEnabled,
  updateTenantConfig,
  type CreateTenantResponse,
  type TenantKey,
  type TenantKeyIssueResponse,
} from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { TenantSummary, useTenant } from "@/lib/tenant";
import {
  scopeApprovalsRead,
  scopeKeysRead,
  scopeKeysRevoke,
  scopeKeysRotate,
  scopeNotificationsRead,
  scopeTenantsWrite,
  scopeTenantsRead,
} from "@/lib/scopes";
import { toast } from "sonner";

export function TenantsPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant, setSelectedTenant, clearSelectedTenant } = useTenant();
  const navigate = useNavigate();
  const [tenants, setTenants] = useState<TenantSummary[]>([]);
  const [notificationStatus, setNotificationStatus] = useState<
    Record<
      string,
      {
        slackEnabled: boolean;
        emailEnabled: boolean;
        configured: boolean;
        error?: string;
      }
    >
  >({});
  const [pendingCounts, setPendingCounts] = useState<Record<string, number>>({});
  const [tenantKeys, setTenantKeys] = useState<TenantKey[]>([]);
  const [keysLoading, setKeysLoading] = useState(false);
  const [keysError, setKeysError] = useState("");
  const [keysActionError, setKeysActionError] = useState("");
  const [keysActionLoading, setKeysActionLoading] = useState<string | null>(null);
  const [issuedKey, setIssuedKey] = useState<TenantKeyIssueResponse | null>(null);
  const [createdTenant, setCreatedTenant] = useState<CreateTenantResponse | null>(null);
  const [tenantActionError, setTenantActionError] = useState("");
  const [tenantActionLoading, setTenantActionLoading] = useState<string | null>(null);
  const [newTenantName, setNewTenantName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const canReadTenants = hasScope(scopeTenantsRead);
  const canWriteTenants = hasScope(scopeTenantsWrite);
  const canReadNotifications = hasScope(scopeNotificationsRead);
  const canReadApprovals = hasScope(scopeApprovalsRead);
  const canReadKeys = hasScope(scopeKeysRead);
  const canRotateKeys = hasScope(scopeKeysRotate);
  const canRevokeKeys = hasScope(scopeKeysRevoke);
  const selectedTenantID = selectedTenant?.tenant_id;

  const refreshTenants = useCallback(async (): Promise<TenantSummary[]> => {
    if (!adminKey || !canReadTenants) {
      setTenants([]);
      setLoading(false);
      return [];
    }

    setLoading(true);
    setError("");
    try {
      const data = await listTenants({ adminKey });
      setTenants(data);
      return data;
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load tenants.");
      return [];
    } finally {
      setLoading(false);
    }
  }, [adminKey, canReadTenants]);

  useEffect(() => {
    void refreshTenants();
  }, [refreshTenants]);

  useEffect(() => {
    if (!adminKey || !canReadNotifications || tenants.length === 0) {
      setNotificationStatus({});
      return;
    }
    let mounted = true;
    const loadStatuses = async () => {
      const results = await Promise.allSettled(
        tenants.map(async (tenant) => {
          const config = await getNotificationConfig({ adminKey }, tenant.tenant_id);
          return { tenantId: tenant.tenant_id, config };
        })
      );
      if (!mounted) {
        return;
      }
      const next: typeof notificationStatus = {};
      results.forEach((result, index) => {
        const tenantId = tenants[index]?.tenant_id;
        if (!tenantId) return;
        if (result.status === "fulfilled") {
          const config = result.value.config;
          next[tenantId] = {
            slackEnabled: config.slack_webhook_enabled || config.slack_bot_enabled,
            emailEnabled: config.email_enabled,
            configured: config.slack_webhook_configured || config.slack_bot_configured || config.email_configured,
          };
        } else {
          const message = result.reason instanceof Error ? result.reason.message : String(result.reason ?? "");
          if (message.includes("notification config not found")) {
            next[tenantId] = {
              slackEnabled: false,
              emailEnabled: false,
              configured: false,
            };
            return;
          }
          next[tenantId] = {
            slackEnabled: false,
            emailEnabled: false,
            configured: false,
            error: message,
          };
        }
      });
      setNotificationStatus(next);
    };
    loadStatuses();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenants, canReadNotifications]);

  useEffect(() => {
    if (!adminKey || !canReadApprovals || tenants.length === 0) {
      setPendingCounts({});
      return;
    }
    let mounted = true;
    const loadCounts = async () => {
      const results = await Promise.allSettled(
        tenants.map(async (tenant) => {
          const response = await getPendingApprovalsCount({ adminKey }, tenant.tenant_id);
          return { tenantId: tenant.tenant_id, count: response.pending_count };
        })
      );
      if (!mounted) {
        return;
      }
      const next: Record<string, number> = {};
      results.forEach((result, index) => {
        const tenantId = tenants[index]?.tenant_id;
        if (!tenantId) return;
        if (result.status === "fulfilled") {
          next[tenantId] = result.value.count;
        }
      });
      setPendingCounts(next);
    };
    loadCounts();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenants, canReadApprovals]);

  const refreshTenantKeys = useCallback(async () => {
    if (!adminKey || !selectedTenantID || !canReadKeys) {
      setTenantKeys([]);
      setKeysLoading(false);
      setKeysError("");
      return;
    }
    setKeysLoading(true);
    setKeysError("");
    try {
      const keys = await listTenantKeys({ adminKey }, selectedTenantID);
      setTenantKeys(keys);
    } catch (err) {
      setKeysError(err instanceof Error ? err.message : "Failed to load tenant keys.");
    } finally {
      setKeysLoading(false);
    }
  }, [adminKey, selectedTenantID, canReadKeys]);

  useEffect(() => {
    void refreshTenantKeys();
  }, [refreshTenantKeys]);

  const handleCreateKey = async () => {
    if (!adminKey || !selectedTenantID || !canRotateKeys) {
      return;
    }
    setKeysActionError("");
    setKeysActionLoading("create");
    try {
      const issued = await createTenantKey({ adminKey }, selectedTenantID);
      setIssuedKey(issued);
      toast.success("Tenant key created");
      if (canReadKeys) {
        await refreshTenantKeys();
      }
    } catch (err) {
      setKeysActionError(err instanceof Error ? err.message : "Failed to create tenant key.");
    } finally {
      setKeysActionLoading(null);
    }
  };

  const handleRotateKeys = async () => {
    if (!adminKey || !selectedTenantID || !canRotateKeys) {
      return;
    }
    setKeysActionError("");
    setKeysActionLoading("rotate");
    try {
      const issued = await rotateTenantKey({ adminKey }, selectedTenantID);
      setIssuedKey(issued);
      toast.success("Tenant keys rotated");
      if (canReadKeys) {
        await refreshTenantKeys();
      }
    } catch (err) {
      setKeysActionError(err instanceof Error ? err.message : "Failed to rotate tenant keys.");
    } finally {
      setKeysActionLoading(null);
    }
  };

  const handleRevokeKey = async (keyID: string, keyPrefix: string) => {
    if (!adminKey || !selectedTenantID || !canRevokeKeys) {
      return;
    }
    if (!window.confirm(`Revoke key ${keyPrefix}?`)) {
      return;
    }
    setKeysActionError("");
    setKeysActionLoading(`revoke:${keyID}`);
    try {
      await revokeTenantKey({ adminKey }, selectedTenantID, keyID);
      toast.success("Tenant key revoked", { description: keyPrefix });
      if (canReadKeys) {
        await refreshTenantKeys();
      }
    } catch (err) {
      setKeysActionError(err instanceof Error ? err.message : "Failed to revoke tenant key.");
    } finally {
      setKeysActionLoading(null);
    }
  };

  const handleCreateTenant = async () => {
    if (!adminKey || !canWriteTenants) return;
    if (!newTenantName.trim()) {
      setTenantActionError("Tenant name is required.");
      return;
    }
    setTenantActionError("");
    setTenantActionLoading("create");
    try {
      const created = await createTenant({ adminKey }, { name: newTenantName.trim() });
      setCreatedTenant(created);
      setNewTenantName("");
      setSelectedTenant({ tenant_id: created.tenant_id, name: created.name });
      toast.success("Tenant created", { description: created.tenant_id });
      await refreshTenants();
    } catch (err) {
      setTenantActionError(parseTenantError(err));
    } finally {
      setTenantActionLoading(null);
    }
  };

  const handleRenameTenant = async (tenant: TenantSummary) => {
    if (!adminKey || !canWriteTenants) return;
    const nextName = window.prompt(`Rename tenant ${tenant.tenant_id}`, tenant.name ?? "");
    if (nextName === null) return;
    if (!nextName.trim()) {
      setTenantActionError("Tenant name cannot be empty.");
      return;
    }

    setTenantActionError("");
    setTenantActionLoading(`rename:${tenant.tenant_id}`);
    try {
      await updateTenantConfig(
        { adminKey },
        tenant.tenant_id,
        { name: nextName.trim() }
      );
      toast.success("Tenant updated", { description: tenant.tenant_id });
      if (selectedTenant?.tenant_id === tenant.tenant_id) {
        setSelectedTenant({
          ...selectedTenant,
          name: nextName.trim(),
        });
      }
      await refreshTenants();
    } catch (err) {
      setTenantActionError(parseTenantError(err));
    } finally {
      setTenantActionLoading(null);
    }
  };

  const handleSetTenantEnabled = async (tenant: TenantSummary, enabled: boolean) => {
    if (!adminKey || !canWriteTenants) return;
    setTenantActionError("");
    setTenantActionLoading(`${enabled ? "enable" : "disable"}:${tenant.tenant_id}`);
    try {
      await setTenantEnabled({ adminKey }, tenant.tenant_id, enabled);
      toast.success(enabled ? "Tenant enabled" : "Tenant disabled", { description: tenant.tenant_id });
      await refreshTenants();
    } catch (err) {
      setTenantActionError(parseTenantError(err));
    } finally {
      setTenantActionLoading(null);
    }
  };

  const handleDeleteTenant = async (tenant: TenantSummary) => {
    if (!adminKey || !canWriteTenants) return;
    const confirmed = window.confirm(
      `Soft-delete tenant ${tenant.tenant_id}? This hides it from UI/API by default and disables access.`
    );
    if (!confirmed) return;

    setTenantActionError("");
    setTenantActionLoading(`delete:${tenant.tenant_id}`);
    try {
      await deleteTenant({ adminKey }, tenant.tenant_id);
      toast.success("Tenant deleted", { description: tenant.tenant_id });
      const updated = await refreshTenants();
      if (selectedTenant?.tenant_id === tenant.tenant_id) {
        if (updated.length > 0) {
          setSelectedTenant(updated[0]);
        } else {
          clearSelectedTenant();
        }
      }
    } catch (err) {
      setTenantActionError(parseTenantError(err));
    } finally {
      setTenantActionLoading(null);
    }
  };

  const legendFlags = useMemo(() => {
    const values = Object.values(notificationStatus);
    return {
      showSlackOn: values.some((item) => item.configured && item.slackEnabled),
      showSlackOff: values.some((item) => item.configured && !item.slackEnabled),
      showEmailOn: values.some((item) => item.configured && item.emailEnabled),
      showEmailOff: values.some((item) => item.configured && !item.emailEnabled),
      showNotConfigured: values.some((item) => !item.configured && !item.error),
      showUnknown: values.some((item) => item.error),
    };
  }, [notificationStatus]);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tenants</CardTitle>
          <CardDescription>Select a tenant to manage policies and evidence.</CardDescription>
          {canReadNotifications && tenants.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>Notification status:</span>
              {legendFlags.showSlackOn ? <Badge variant="default">Slack On</Badge> : null}
              {legendFlags.showSlackOff ? <Badge variant="outline">Slack Off</Badge> : null}
              {legendFlags.showEmailOn ? <Badge variant="default">Email On</Badge> : null}
              {legendFlags.showEmailOff ? <Badge variant="outline">Email Off</Badge> : null}
              {legendFlags.showNotConfigured ? <Badge variant="outline">Not configured</Badge> : null}
              {legendFlags.showUnknown ? <Badge variant="outline">Unknown</Badge> : null}
            </div>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-4">
          {createdTenant ? (
            <Alert>
              <AlertDescription>
                <div className="space-y-2">
                  <div className="text-sm font-medium">
                    Tenant created: {createdTenant.tenant_id}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    Initial tenant API key (shown once)
                  </div>
                  <code className="block overflow-auto rounded border bg-muted px-2 py-1 text-xs">
                    {createdTenant.api_key}
                  </code>
                  <div className="flex justify-end">
                    <Button size="sm" variant="outline" onClick={() => setCreatedTenant(null)}>
                      Dismiss
                    </Button>
                  </div>
                </div>
              </AlertDescription>
            </Alert>
          ) : null}

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {tenantActionError ? (
            <Alert variant="destructive">
              <AlertDescription>{tenantActionError}</AlertDescription>
            </Alert>
          ) : null}
          {!canReadTenants ? (
            <Alert>
              <AlertDescription>Missing scope: {scopeTenantsRead}</AlertDescription>
            </Alert>
          ) : null}
          {canWriteTenants ? (
            <div className="flex flex-wrap items-end gap-2 rounded-md border p-3">
              <div className="min-w-56 flex-1">
                <div className="mb-1 text-xs text-muted-foreground">Create tenant</div>
                <Input
                  placeholder="Tenant name"
                  value={newTenantName}
                  onChange={(event) => setNewTenantName(event.target.value)}
                />
              </div>
              <Button
                size="sm"
                onClick={() => void handleCreateTenant()}
                disabled={tenantActionLoading === "create"}
              >
                {tenantActionLoading === "create" ? "Creating..." : "Create tenant"}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void refreshTenants()}
                disabled={loading}
              >
                Refresh
              </Button>
            </div>
          ) : null}
          {loading ? (
            <div className="text-sm text-muted-foreground">Loading tenants...</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tenant</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Active policy</TableHead>
                  {canReadNotifications ? <TableHead>Notifications</TableHead> : null}
                  {canReadApprovals ? <TableHead>Pending approvals</TableHead> : null}
                  <TableHead>Tools</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.map((tenant) => (
                  <TableRow key={tenant.tenant_id}>
                    <TableCell className="font-medium">{tenant.tenant_id}</TableCell>
                    <TableCell>{tenant.name ?? "—"}</TableCell>
                    <TableCell>{tenant.active_policy_version ?? "—"}</TableCell>
                    {canReadNotifications ? (
                      <TableCell>
                        {notificationStatus[tenant.tenant_id]?.error ? (
                          <Badge variant="outline">Unavailable</Badge>
                        ) : notificationStatus[tenant.tenant_id]?.configured ? (
                          <div className="flex flex-wrap gap-2">
                            <Badge variant={notificationStatus[tenant.tenant_id]?.slackEnabled ? "default" : "outline"}>
                              Slack {notificationStatus[tenant.tenant_id]?.slackEnabled ? "On" : "Off"}
                            </Badge>
                            <Badge variant={notificationStatus[tenant.tenant_id]?.emailEnabled ? "default" : "outline"}>
                              Email {notificationStatus[tenant.tenant_id]?.emailEnabled ? "On" : "Off"}
                            </Badge>
                          </div>
                        ) : (
                          <Badge variant="outline">None</Badge>
                        )}
                      </TableCell>
                    ) : null}
                    {canReadApprovals ? (
                      <TableCell>
                        {pendingCounts[tenant.tenant_id] !== undefined ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="px-2"
                            onClick={() => {
                              setSelectedTenant(tenant);
                              navigate("/approvals");
                            }}
                          >
                            <Badge variant={pendingCounts[tenant.tenant_id] > 0 ? "default" : "outline"}>
                              {pendingCounts[tenant.tenant_id]}
                            </Badge>
                          </Button>
                        ) : (
                          "—"
                        )}
                      </TableCell>
                    ) : null}
                    <TableCell>{tenant.tool_count ?? "—"}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-wrap justify-end gap-2">
                        {canWriteTenants ? (
                          <>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => void handleRenameTenant(tenant)}
                              disabled={Boolean(tenantActionLoading)}
                            >
                              {tenantActionLoading === `rename:${tenant.tenant_id}` ? "Renaming..." : "Rename"}
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => void handleSetTenantEnabled(tenant, true)}
                              disabled={Boolean(tenantActionLoading)}
                            >
                              {tenantActionLoading === `enable:${tenant.tenant_id}` ? "Enabling..." : "Enable"}
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => void handleSetTenantEnabled(tenant, false)}
                              disabled={Boolean(tenantActionLoading)}
                            >
                              {tenantActionLoading === `disable:${tenant.tenant_id}` ? "Disabling..." : "Disable"}
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => void handleDeleteTenant(tenant)}
                              disabled={Boolean(tenantActionLoading)}
                            >
                              {tenantActionLoading === `delete:${tenant.tenant_id}` ? "Deleting..." : "Delete"}
                            </Button>
                          </>
                        ) : null}
                        <Button
                          size="sm"
                          variant={selectedTenant?.tenant_id === tenant.tenant_id ? "secondary" : "outline"}
                          onClick={() => setSelectedTenant(tenant)}
                        >
                          {selectedTenant?.tenant_id === tenant.tenant_id ? "Selected" : "Select"}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={!canReadNotifications}
                          onClick={() => {
                            setSelectedTenant(tenant);
                            navigate("/notifications");
                          }}
                        >
                          Notifications
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Tenant Keys</CardTitle>
          <CardDescription>
            {selectedTenantID
              ? `Manage keys for ${selectedTenantID}. New API keys are shown once after issuance.`
              : "Select a tenant above to manage keys."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {issuedKey ? (
            <Alert>
              <AlertDescription>
                <div className="space-y-2">
                  <div className="text-sm font-medium">New tenant key (shown once)</div>
                  <code className="block overflow-auto rounded border bg-muted px-2 py-1 text-xs">
                    {issuedKey.api_key}
                  </code>
                  <div className="flex justify-end">
                    <Button size="sm" variant="outline" onClick={() => setIssuedKey(null)}>
                      Dismiss
                    </Button>
                  </div>
                </div>
              </AlertDescription>
            </Alert>
          ) : null}

          {keysError ? (
            <Alert variant="destructive">
              <AlertDescription>{keysError}</AlertDescription>
            </Alert>
          ) : null}

          {keysActionError ? (
            <Alert variant="destructive">
              <AlertDescription>{keysActionError}</AlertDescription>
            </Alert>
          ) : null}

          {!selectedTenantID ? (
            <div className="text-sm text-muted-foreground">Select a tenant to view and manage keys.</div>
          ) : (
            <>
              {!canReadKeys ? (
                <Alert>
                  <AlertDescription>Missing scope: {scopeKeysRead}</AlertDescription>
                </Alert>
              ) : null}
              <div className="flex flex-wrap justify-end gap-2">
                {canReadKeys ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void refreshTenantKeys()}
                    disabled={keysLoading}
                  >
                    Refresh
                  </Button>
                ) : null}
                {canRotateKeys ? (
                  <>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void handleCreateKey()}
                      disabled={Boolean(keysActionLoading)}
                    >
                      {keysActionLoading === "create" ? "Creating..." : "Add key"}
                    </Button>
                    <Button
                      size="sm"
                      onClick={() => void handleRotateKeys()}
                      disabled={Boolean(keysActionLoading)}
                    >
                      {keysActionLoading === "rotate" ? "Rotating..." : "Rotate keys"}
                    </Button>
                  </>
                ) : null}
              </div>

              {canReadKeys ? (
                keysLoading ? (
                  <div className="text-sm text-muted-foreground">Loading tenant keys...</div>
                ) : tenantKeys.length === 0 ? (
                  <div className="text-sm text-muted-foreground">
                    No API keys yet. Create a key so agents and admins can
                    authenticate for this tenant.
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Key ID</TableHead>
                        <TableHead>Prefix</TableHead>
                        <TableHead>Created</TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Revoked</TableHead>
                        <TableHead></TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {tenantKeys.map((key) => {
                        const isRevoked = Boolean(key.revoked_at);
                        return (
                          <TableRow key={key.key_id}>
                            <TableCell className="font-mono text-xs">{key.key_id}</TableCell>
                            <TableCell className="font-mono text-xs">{key.key_prefix}</TableCell>
                            <TableCell>{new Date(key.created_at).toLocaleString()}</TableCell>
                            <TableCell>
                              <Badge variant={isRevoked ? "outline" : "default"}>
                                {isRevoked ? "Revoked" : "Active"}
                              </Badge>
                            </TableCell>
                            <TableCell>{key.revoked_at ? new Date(key.revoked_at).toLocaleString() : "—"}</TableCell>
                            <TableCell className="text-right">
                              {canRevokeKeys ? (
                                <Button
                                  size="sm"
                                  variant="outline"
                                  onClick={() => void handleRevokeKey(key.key_id, key.key_prefix)}
                                  disabled={isRevoked || Boolean(keysActionLoading)}
                                >
                                  {keysActionLoading === `revoke:${key.key_id}` ? "Revoking..." : "Revoke"}
                                </Button>
                              ) : (
                                <span className="text-xs text-muted-foreground">No revoke scope</span>
                              )}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                )
              ) : null}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function parseTenantError(err: unknown): string {
  const message = err instanceof Error ? err.message.trim() : String(err ?? "").trim();
  if (!message) {
    return "Tenant operation failed.";
  }

  try {
    const parsed = JSON.parse(message) as { error?: string };
    if (parsed.error) {
      return parsed.error;
    }
  } catch {
    // Ignore parse failure and return original message.
  }
  return message;
}
