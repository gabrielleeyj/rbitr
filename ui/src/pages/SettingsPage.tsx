import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getSettings, setAdminWriteLock, setDefaultApprovalTTL } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { toast } from "sonner";

export function SettingsPage() {
  const { adminKey } = useAdminKey();
  const [locked, setLocked] = useState(false);
  const [defaultTTLMinutes, setDefaultTTLMinutes] = useState(15);
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
        const data = await getSettings({ adminKey });
        if (!mounted) return;
        setLocked(Boolean(data.admin_write_lock));
        if (data.default_approval_ttl_seconds) {
          setDefaultTTLMinutes(Math.round(data.default_approval_ttl_seconds / 60));
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
  }, [adminKey]);

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

  return (
    <div className="space-y-6">
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
          <CardDescription>Recent admin changes appear in the Audit tab.</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" asChild>
            <Link to="/audit">View audit log</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
