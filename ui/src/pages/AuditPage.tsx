import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { listAuditEvents } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { Button } from "@/components/ui/button";

export function AuditPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [events, setEvents] = useState<Array<{ audit_event_id: string; action: string; resource_type: string; resource_id?: string; actor_display?: string; created_at: string }>>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = async () => {
    if (!adminKey || !tenantId) return;
    const data = await listAuditEvents({ adminKey }, tenantId, 50);
    setEvents(data);
  };

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !tenantId) {
        setLoading(false);
        return;
      }
      try {
        const data = await listAuditEvents({ adminKey }, tenantId, 50);
        if (!mounted) return;
        setEvents(data);
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load audit events.");
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Audit events</CardTitle>
        <CardDescription>Review recent admin changes for the selected tenant.</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {!tenantId ? (
          <div className="text-sm text-muted-foreground">Select a tenant to view audit events.</div>
        ) : loading ? (
          <div className="text-sm text-muted-foreground">Loading audit events...</div>
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
              <TableHead>Time</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Resource</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.length === 0 ? (
              <TableRow>
                <TableCell className="text-muted-foreground">—</TableCell>
                <TableCell className="text-muted-foreground">No audit events yet</TableCell>
                <TableCell className="text-muted-foreground">—</TableCell>
                <TableCell className="text-muted-foreground">—</TableCell>
              </TableRow>
            ) : (
              events.map((event) => (
                <TableRow key={event.audit_event_id}>
                  <TableCell>{event.created_at ? new Date(event.created_at).toLocaleString() : "—"}</TableCell>
                  <TableCell>{event.action}</TableCell>
                  <TableCell>{event.actor_display ?? "—"}</TableCell>
                  <TableCell>
                    {event.resource_type}
                    {event.resource_id ? ` · ${event.resource_id}` : ""}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        </div>
        )}
      </CardContent>
    </Card>
  );
}
