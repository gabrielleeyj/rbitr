import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getSettings, setAdminWriteLock } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { toast } from "sonner";

export function SettingsPage() {
  const { adminKey } = useAdminKey();
  const [locked, setLocked] = useState(false);
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
