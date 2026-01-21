import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { deleteRiskOverride, listRiskOverrides, upsertRiskOverride } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";

export function RiskOverridesPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [actionType, setActionType] = useState("");
  const [actionRisk, setActionRisk] = useState("HIGH");
  const [overrides, setOverrides] = useState<Array<{ action_type: string; action_risk: string; updated_at: string }>>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");

  const refresh = async () => {
    if (!adminKey || !tenantId) return;
    const data = await listRiskOverrides({ adminKey }, tenantId);
    setOverrides(data);
  };

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !tenantId) {
        setLoading(false);
        return;
      }
      try {
        const data = await listRiskOverrides({ adminKey }, tenantId);
        if (!mounted) return;
        setOverrides(data);
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load overrides.");
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId]);

  const handleUpsert = async () => {
    if (!adminKey || !tenantId || !actionType) return;
    setActionError("");
    try {
      await upsertRiskOverride({ adminKey }, tenantId, actionType, actionRisk);
      setActionType("");
      await refresh();
      toast.success("Risk override saved", { description: actionType });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to upsert override.");
    }
  };

  const handleDelete = async (type?: string) => {
    if (!adminKey || !tenantId) return;
    const target = type ?? actionType;
    if (!target) return;
    setActionError("");
    try {
      await deleteRiskOverride({ adminKey }, tenantId, target);
      if (!type) {
        setActionType("");
      }
      await refresh();
      toast.success("Risk override deleted", { description: target });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to delete override.");
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Risk overrides</CardTitle>
        <CardDescription>Upsert or remove action risk overrides.</CardDescription>
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
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="action-type">Action type</Label>
            <Input
              id="action-type"
              value={actionType}
              onChange={(event) => setActionType(event.target.value)}
              placeholder="DATA.EXPORT"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="risk">Risk</Label>
            <Select value={actionRisk} onValueChange={setActionRisk}>
              <SelectTrigger id="risk">
                <SelectValue placeholder="Select risk" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="LOW">LOW</SelectItem>
                <SelectItem value="MEDIUM">MEDIUM</SelectItem>
                <SelectItem value="HIGH">HIGH</SelectItem>
                <SelectItem value="CRITICAL">CRITICAL</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-end gap-2">
            <Button onClick={handleUpsert} disabled={!tenantId}>
              Upsert
            </Button>
            <Button variant="outline" onClick={() => handleDelete()} disabled={!tenantId}>
              Delete
            </Button>
          </div>
        </div>
        {!tenantId ? (
          <div className="text-sm text-muted-foreground">Select a tenant to view overrides.</div>
        ) : loading ? (
          <div className="text-sm text-muted-foreground">Loading overrides...</div>
        ) : overrides.length === 0 ? (
          <div className="text-sm text-muted-foreground">No overrides configured.</div>
        ) : (
          <div className="space-y-3">
            <div className="flex justify-end">
              <Button variant="outline" size="sm" onClick={refresh}>
                Refresh
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Action type</TableHead>
                  <TableHead>Risk</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {overrides.map((override) => (
                  <TableRow key={override.action_type}>
                    <TableCell className="font-medium">{override.action_type}</TableCell>
                    <TableCell>{override.action_risk}</TableCell>
                    <TableCell>{override.updated_at ? new Date(override.updated_at).toLocaleString() : "—"}</TableCell>
                    <TableCell className="text-right">
                      <Button size="sm" variant="outline" onClick={() => handleDelete(override.action_type)}>
                        Delete
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
