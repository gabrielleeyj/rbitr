import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getSettings,
  listTools,
  setAdminWriteLock,
  setAuditRetentionDays,
  setDisableXTenantKey as updateDisableXTenantKey,
  setDefaultApprovalTTL,
  setDefaultRateLimitConfig,
  setEnforcementMode,
  setFeatureArgConstraints as updateFeatureArgConstraints,
  setFeatureRateLimiting as updateFeatureRateLimiting,
  setMCPPassthroughUpstreamTool,
  type ToolConfig,
} from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";
import { scopeAuditRead, scopeSettingsRead, scopeSettingsWrite, scopeToolsRead } from "@/lib/scopes";

type RateLimitScope = "tenant" | "tenant_agent" | "tenant_tool" | "tenant_agent_tool";

export function SettingsPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const [locked, setLocked] = useState(false);
  const [defaultTTLMinutes, setDefaultTTLMinutes] = useState(15);
  const [auditRetentionDays, setAuditRetentionDaysState] = useState(365);
  const [enforcementMode, setEnforcementModeState] = useState<"enforce" | "shadow">("enforce");
  const [disableXTenantKey, setDisableXTenantKey] = useState(false);
  const [featureRateLimiting, setFeatureRateLimiting] = useState(false);
  const [featureArgConstraints, setFeatureArgConstraints] = useState(false);
  const [rateLimitPerMinute, setRateLimitPerMinute] = useState(60);
  const [rateLimitPerDay, setRateLimitPerDay] = useState(10000);
  const [rateLimitScope, setRateLimitScope] = useState<RateLimitScope>("tenant_agent_tool");
  const [mcpPassthroughUpstreamToolID, setMCPPassthroughUpstreamToolID] = useState("");
  const [mcpUpstreamTools, setMCPUpstreamTools] = useState<ToolConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const canReadSettings = hasScope(scopeSettingsRead);
  const canWriteSettings = hasScope(scopeSettingsWrite);
  const canReadAudit = hasScope(scopeAuditRead);
  const canReadTools = hasScope(scopeToolsRead);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !canReadSettings) {
        setLoading(false);
        return;
      }
      try {
        const settingsPromise = getSettings({ adminKey }, selectedTenant?.tenant_id);
        const toolsPromise =
          selectedTenant?.tenant_id && canReadTools
            ? listTools({ adminKey }, selectedTenant.tenant_id)
            : Promise.resolve([]);
        const [data, tools] = await Promise.all([settingsPromise, toolsPromise]);
        if (!mounted) return;
        setLocked(Boolean(data.admin_write_lock));
        if (data.default_approval_ttl_seconds) {
          setDefaultTTLMinutes(Math.round(data.default_approval_ttl_seconds / 60));
        }
        if (data.audit_retention_days) {
          setAuditRetentionDaysState(data.audit_retention_days);
        }
        setDisableXTenantKey(Boolean(data.disable_x_tenant_key));
        setFeatureRateLimiting(Boolean(data.feature_rate_limiting));
        setFeatureArgConstraints(Boolean(data.feature_arg_constraints));
        setRateLimitPerMinute(Number(data.default_rate_limit_per_minute) > 0 ? Number(data.default_rate_limit_per_minute) : 60);
        setRateLimitPerDay(Number(data.default_rate_limit_per_day) > 0 ? Number(data.default_rate_limit_per_day) : 10000);
        const nextScope = data.default_rate_limit_scope;
        if (
          nextScope === "tenant" ||
          nextScope === "tenant_agent" ||
          nextScope === "tenant_tool" ||
          nextScope === "tenant_agent_tool"
        ) {
          setRateLimitScope(nextScope);
        } else {
          setRateLimitScope("tenant_agent_tool");
        }
        if (data.enforcement_mode === "shadow") {
          setEnforcementModeState("shadow");
        } else {
          setEnforcementModeState("enforce");
        }
        setMCPPassthroughUpstreamToolID(data.mcp_passthrough_upstream_tool_id ?? "");
        setMCPUpstreamTools(
          tools.filter((tool) => {
            const upstream = tool.mcp?.upstream_url?.trim();
            return Boolean(upstream);
          })
        );
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load settings.");
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, selectedTenant?.tenant_id, canReadSettings, canReadTools]);

  const handleToggle = async (value: boolean) => {
    if (!adminKey || !canWriteSettings) return;
    setActionError("");
    setLocked(value);
    try {
      await setAdminWriteLock({ adminKey }, value);
      toast.success("Admin write lock updated", { description: value ? "Enabled" : "Disabled" });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update write lock.");
    }
  };

  const handleTTLUpdate = async () => {
    if (!adminKey || !canWriteSettings) return;
    setActionError("");
    const seconds = Math.round(defaultTTLMinutes * 60);
    try {
      await setDefaultApprovalTTL({ adminKey }, seconds);
      toast.success("Default approval TTL updated", { description: `${defaultTTLMinutes} minutes` });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update TTL.");
    }
  };

  const handleAuditRetentionUpdate = async () => {
    if (!adminKey || !canWriteSettings) return;
    setActionError("");
    try {
      await setAuditRetentionDays({ adminKey }, auditRetentionDays);
      toast.success("Audit retention updated", { description: `${auditRetentionDays} days` });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update audit retention.");
    }
  };

  const handleEnforcementModeToggle = async (value: boolean) => {
    if (!adminKey || !selectedTenant?.tenant_id || !canWriteSettings) return;
    const nextMode = value ? "shadow" : "enforce";
    setActionError("");
    setEnforcementModeState(nextMode);
    try {
      await setEnforcementMode({ adminKey }, selectedTenant.tenant_id, nextMode);
      toast.success("Tenant enforcement mode updated", { description: nextMode === "shadow" ? "Shadow" : "Enforce" });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update enforcement mode.");
      setEnforcementModeState(nextMode === "shadow" ? "enforce" : "shadow");
    }
  };

  const handleMCPPassthroughUpstreamSave = async () => {
    if (!adminKey || !selectedTenant?.tenant_id || !canWriteSettings) return;
    setActionError("");
    try {
      await setMCPPassthroughUpstreamTool(
        { adminKey },
        selectedTenant.tenant_id,
        mcpPassthroughUpstreamToolID
      );
      if (mcpPassthroughUpstreamToolID) {
        toast.success("MCP pass-through upstream updated", {
          description: `Using ${mcpPassthroughUpstreamToolID}`,
        });
      } else {
        toast.success("MCP pass-through upstream cleared", {
          description: "Automatic fallback routing is enabled.",
        });
      }
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update MCP pass-through upstream.");
    }
  };

  const handleDisableXTenantKeyToggle = async (value: boolean) => {
    if (!adminKey || !canWriteSettings) return;
    setActionError("");
    setDisableXTenantKey(value);
    try {
      await updateDisableXTenantKey({ adminKey }, value);
      toast.success("Disable X-Tenant-Key fallback updated", { description: value ? "Enabled" : "Disabled" });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update X-Tenant-Key fallback.");
      setDisableXTenantKey(!value);
    }
  };

  const handleFeatureRateLimitingToggle = async (value: boolean) => {
    if (!adminKey || !canWriteSettings) return;
    setActionError("");
    setFeatureRateLimiting(value);
    try {
      await updateFeatureRateLimiting({ adminKey }, value);
      toast.success("Rate limiting feature updated", { description: value ? "Enabled" : "Disabled" });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update rate limiting feature.");
      setFeatureRateLimiting(!value);
    }
  };

  const handleFeatureArgConstraintsToggle = async (value: boolean) => {
    if (!adminKey || !canWriteSettings) return;
    setActionError("");
    setFeatureArgConstraints(value);
    try {
      await updateFeatureArgConstraints({ adminKey }, value);
      toast.success("Argument constraints feature updated", { description: value ? "Enabled" : "Disabled" });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update argument constraints feature.");
      setFeatureArgConstraints(!value);
    }
  };

  const rateLimitValidationError = (() => {
    if (!Number.isInteger(rateLimitPerMinute) || rateLimitPerMinute <= 0) {
      return "Per-minute limit must be a positive integer.";
    }
    if (!Number.isInteger(rateLimitPerDay) || rateLimitPerDay <= 0) {
      return "Per-day limit must be a positive integer.";
    }
    if (rateLimitPerDay < rateLimitPerMinute) {
      return "Per-day limit must be greater than or equal to per-minute limit.";
    }
    return "";
  })();

  const handleRateLimitConfigSave = async () => {
    if (!adminKey || !canWriteSettings || rateLimitValidationError) return;
    setActionError("");
    try {
      await setDefaultRateLimitConfig(
        { adminKey },
        {
          per_minute: rateLimitPerMinute,
          per_day: rateLimitPerDay,
          scope: rateLimitScope,
        }
      );
      toast.success("Default rate limit config updated", {
        description: `${rateLimitPerMinute}/min, ${rateLimitPerDay}/day, scope ${rateLimitScope}`,
      });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update default rate limit config.");
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tenant settings</CardTitle>
          <CardDescription>
            {selectedTenant ? `Selected tenant: ${selectedTenant.tenant_id}` : "Select a tenant to configure tenant-level controls."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {actionError ? (
            <Alert variant="destructive">
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
          {!canReadSettings ? (
            <Alert>
              <AlertDescription>Missing scope: {scopeSettingsRead}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex items-center justify-between">
            <Label htmlFor="enforcement-mode" className="text-sm">
              Shadow mode (evaluate deny, execute anyway)
            </Label>
            <Switch
              id="enforcement-mode"
              checked={enforcementMode === "shadow"}
              onCheckedChange={handleEnforcementModeToggle}
              disabled={loading || !selectedTenant || !canWriteSettings}
            />
          </div>
          <div className="text-xs text-muted-foreground">
            In shadow mode, DENY decisions are logged with explainability metadata but calls still execute. Approval flows remain enforced.
          </div>
          <div className="space-y-2">
            <Label htmlFor="mcp-passthrough-upstream" className="text-sm">
              MCP pass-through upstream tool
            </Label>
            <div className="flex flex-col gap-2 md:flex-row md:items-center">
              <select
                id="mcp-passthrough-upstream"
                value={mcpPassthroughUpstreamToolID}
                onChange={(event) => setMCPPassthroughUpstreamToolID(event.target.value)}
                className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm md:max-w-md"
                disabled={loading || !selectedTenant || !canWriteSettings || !canReadTools}
              >
                <option value="">Automatic fallback (first MCP tool)</option>
                {mcpUpstreamTools.map((tool) => (
                  <option key={tool.tool_id} value={tool.tool_id}>
                    {tool.tool_id}
                  </option>
                ))}
              </select>
              <Button
                variant="outline"
                onClick={handleMCPPassthroughUpstreamSave}
                disabled={loading || !selectedTenant || !canWriteSettings || !canReadTools}
              >
                Save
              </Button>
            </div>
            {!canReadTools ? (
              <div className="text-xs text-muted-foreground">Missing scope: {scopeToolsRead}</div>
            ) : (
              <div className="text-xs text-muted-foreground">
                Only MCP tools with a configured upstream URL are listed.
              </div>
            )}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Gateway runtime controls</CardTitle>
          <CardDescription>
            System-wide controls for tenant auth and policy enforcement guards.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!canReadSettings ? (
            <Alert>
              <AlertDescription>Missing scope: {scopeSettingsRead}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex items-center justify-between">
            <Label htmlFor="disable-x-tenant-key" className="text-sm">
              Disable X-Tenant-Key fallback
            </Label>
            <Switch
              id="disable-x-tenant-key"
              checked={disableXTenantKey}
              onCheckedChange={handleDisableXTenantKeyToggle}
              disabled={loading || !canWriteSettings}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="feature-rate-limiting" className="text-sm">
              Feature: Rate limiting
            </Label>
            <Switch
              id="feature-rate-limiting"
              checked={featureRateLimiting}
              onCheckedChange={handleFeatureRateLimitingToggle}
              disabled={loading || !canWriteSettings}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="feature-arg-constraints" className="text-sm">
              Feature: Argument constraints
            </Label>
            <Switch
              id="feature-arg-constraints"
              checked={featureArgConstraints}
              onCheckedChange={handleFeatureArgConstraintsToggle}
              disabled={loading || !canWriteSettings}
            />
          </div>
          {featureRateLimiting ? (
            <div className="rounded-md border border-border p-3 space-y-3">
              <div className="text-sm font-medium">Default rate limits</div>
              <div className="grid gap-3 md:grid-cols-3">
                <div className="space-y-1">
                  <Label htmlFor="rate-limit-per-minute" className="text-xs text-muted-foreground">
                    Per minute
                  </Label>
                  <input
                    id="rate-limit-per-minute"
                    type="number"
                    min={1}
                    value={rateLimitPerMinute}
                    onChange={(event) => setRateLimitPerMinute(Number(event.target.value) || 0)}
                    className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
                    disabled={loading || !canWriteSettings}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="rate-limit-per-day" className="text-xs text-muted-foreground">
                    Per day
                  </Label>
                  <input
                    id="rate-limit-per-day"
                    type="number"
                    min={1}
                    value={rateLimitPerDay}
                    onChange={(event) => setRateLimitPerDay(Number(event.target.value) || 0)}
                    className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
                    disabled={loading || !canWriteSettings}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="rate-limit-scope" className="text-xs text-muted-foreground">
                    Scope
                  </Label>
                  <select
                    id="rate-limit-scope"
                    value={rateLimitScope}
                    onChange={(event) => setRateLimitScope(event.target.value as RateLimitScope)}
                    className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
                    disabled={loading || !canWriteSettings}
                  >
                    <option value="tenant_agent_tool">Tenant + Agent + Tool</option>
                    <option value="tenant">Tenant</option>
                    <option value="tenant_agent">Tenant + Agent</option>
                    <option value="tenant_tool">Tenant + Tool</option>
                  </select>
                </div>
              </div>
              {rateLimitValidationError ? (
                <div className="text-xs text-destructive">{rateLimitValidationError}</div>
              ) : (
                <div className="text-xs text-muted-foreground">
                  Applies to default enforcement when policy-level overrides are not provided.
                </div>
              )}
              <div className="flex justify-end">
                <Button
                  variant="outline"
                  onClick={handleRateLimitConfigSave}
                  disabled={loading || !canWriteSettings || Boolean(rateLimitValidationError)}
                >
                  Save default rate limits
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Admin write lock</CardTitle>
          <CardDescription>Freeze all admin writes across tenants.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {actionError ? (
            <Alert variant="destructive">
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex items-center justify-between">
            <Label htmlFor="write-lock" className="text-sm">
              Write lock enabled
            </Label>
            <Switch id="write-lock" checked={locked} onCheckedChange={handleToggle} disabled={loading || !canWriteSettings} />
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Default approval TTL</CardTitle>
          <CardDescription>Fallback expiry used when policies do not specify a TTL.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {actionError ? (
            <Alert variant="destructive">
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <Label htmlFor="approval-ttl" className="text-sm">
              Default TTL (minutes)
            </Label>
            <div className="flex items-center gap-2">
              <input
                id="approval-ttl"
                type="number"
                min={1}
                max={1440}
                value={defaultTTLMinutes}
                onChange={(event) => setDefaultTTLMinutes(Number(event.target.value) || 15)}
                className="h-9 w-24 rounded-md border border-border bg-background px-3 text-sm"
                disabled={loading || !canWriteSettings}
              />
              <Button variant="outline" onClick={handleTTLUpdate} disabled={loading || !canWriteSettings}>
                Save
              </Button>
            </div>
          </div>
          <div className="text-xs text-muted-foreground">Min 1 minute, max 1440 minutes.</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Audit trail</CardTitle>
          <CardDescription>Configure retention and review admin changes.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {actionError ? (
            <Alert variant="destructive">
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <Label htmlFor="audit-retention" className="text-sm">
              Audit retention (days)
            </Label>
            <div className="flex items-center gap-2">
              <input
                id="audit-retention"
                type="number"
                min={30}
                max={3650}
                value={auditRetentionDays}
                onChange={(event) => setAuditRetentionDaysState(Number(event.target.value) || 365)}
                className="h-9 w-24 rounded-md border border-border bg-background px-3 text-sm"
                disabled={loading || !canWriteSettings}
              />
              <Button variant="outline" onClick={handleAuditRetentionUpdate} disabled={loading || !canWriteSettings}>
                Save
              </Button>
            </div>
          </div>
          <div className="text-xs text-muted-foreground">Min 30 days, max 3650 days.</div>
          {canReadAudit ? (
            <Button variant="outline" asChild>
              <Link to="/audit">View audit log</Link>
            </Button>
          ) : (
            <Button variant="outline" disabled>
              View audit log
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
