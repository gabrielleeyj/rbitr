import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getSettings, setAdminWriteLock, setAuditRetentionDays, setDefaultApprovalTTL, setEnforcementMode } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";

export function SettingsPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const [locked, setLocked] = useState(false);
  const [defaultTTLMinutes, setDefaultTTLMinutes] = useState(15);
  const [auditRetentionDays, setAuditRetentionDaysState] = useState(365);
  const [enforcementMode, setEnforcementModeState] = useState<"enforce" | "shadow">("enforce");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey) {
        setLoading(false);
        return;
      }
      try {
        const data = await getSettings({ adminKey }, selectedTenant?.tenant_id);
        if (!mounted) return;
        setLocked(Boolean(data.admin_write_lock));
        if (data.default_approval_ttl_seconds) {
          setDefaultTTLMinutes(Math.round(data.default_approval_ttl_seconds / 60));
        }
        if (data.audit_retention_days) {
          setAuditRetentionDaysState(data.audit_retention_days);
        }
        if (data.enforcement_mode === "shadow") {
          setEnforcementModeState("shadow");
        } else {
          setEnforcementModeState("enforce");
        }
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
  }, [adminKey, selectedTenant?.tenant_id]);

  const handleToggle = async (value: boolean) => {
    if (!adminKey) return;
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
    if (!adminKey) return;
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
    if (!adminKey) return;
    setActionError("");
    try {
      await setAuditRetentionDays({ adminKey }, auditRetentionDays);
      toast.success("Audit retention updated", { description: `${auditRetentionDays} days` });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update audit retention.");
    }
  };

  const handleEnforcementModeToggle = async (value: boolean) => {
    if (!adminKey || !selectedTenant?.tenant_id) return;
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
          <div className="flex items-center justify-between">
            <Label htmlFor="enforcement-mode" className="text-sm">
              Shadow mode (evaluate deny, execute anyway)
            </Label>
            <Switch
              id="enforcement-mode"
              checked={enforcementMode === "shadow"}
              onCheckedChange={handleEnforcementModeToggle}
              disabled={loading || !selectedTenant}
            />
          </div>
          <div className="text-xs text-muted-foreground">
            In shadow mode, DENY decisions are logged with explainability metadata but calls still execute. Approval flows remain enforced.
          </div>
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
            <Switch id="write-lock" checked={locked} onCheckedChange={handleToggle} disabled={loading} />
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
                disabled={loading}
              />
              <Button variant="outline" onClick={handleTTLUpdate} disabled={loading}>
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
                disabled={loading}
              />
              <Button variant="outline" onClick={handleAuditRetentionUpdate} disabled={loading}>
                Save
              </Button>
            </div>
          </div>
          <div className="text-xs text-muted-foreground">Min 30 days, max 3650 days.</div>
          <Button variant="outline" asChild>
            <Link to="/audit">View audit log</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
