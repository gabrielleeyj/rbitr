import { useEffect, useMemo, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { listAuditEvents } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

export function AuditPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [events, setEvents] = useState<
    Array<{
      audit_event_id: string;
      action: string;
      resource_type: string;
      resource_id?: string;
      actor_display?: string;
      actor_id?: string;
      created_at: string;
    }>
  >([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [limit, setLimit] = useState(25);
  const [offset, setOffset] = useState(0);
  const [actionFilter, setActionFilter] = useState("");
  const [resourceTypeFilter, setResourceTypeFilter] = useState("");
  const [actorIDFilter, setActorIDFilter] = useState("");
  const [actionFilterInput, setActionFilterInput] = useState("");
  const [resourceTypeFilterInput, setResourceTypeFilterInput] = useState("");
  const [actorIDFilterInput, setActorIDFilterInput] = useState("");
  const [hasMore, setHasMore] = useState(false);

  const pageNumber = Math.floor(offset / limit) + 1;
  const canPrev = offset > 0;
  const canNext = hasMore;

  const filtersActive = useMemo(
    () => Boolean(actionFilterInput || resourceTypeFilterInput || actorIDFilterInput),
    [actionFilterInput, resourceTypeFilterInput, actorIDFilterInput]
  );

  const refresh = async (nextOffset = offset) => {
    if (!adminKey || !tenantId) return;
    const data = await listAuditEvents(
      { adminKey },
      tenantId,
      {
        limit: limit + 1,
        offset: nextOffset,
        action: actionFilter || undefined,
        resource_type: resourceTypeFilter || undefined,
        actor_id: actorIDFilter || undefined,
      }
    );
    setHasMore(data.length > limit);
    setEvents(data.slice(0, limit));
  };

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !tenantId) {
        setLoading(false);
        return;
      }
      try {
        setLoading(true);
        const data = await listAuditEvents(
          { adminKey },
          tenantId,
          {
            limit: limit + 1,
            offset,
            action: actionFilter || undefined,
            resource_type: resourceTypeFilter || undefined,
            actor_id: actorIDFilter || undefined,
          }
        );
        if (!mounted) return;
        setHasMore(data.length > limit);
        setEvents(data.slice(0, limit));
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
  }, [adminKey, tenantId, limit, offset, actionFilter, resourceTypeFilter, actorIDFilter]);

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
          <div className="grid gap-3 md:grid-cols-4">
            <div>
              <div className="text-xs text-muted-foreground">Action</div>
              <Input
                value={actionFilterInput}
                onChange={(event) => setActionFilterInput(event.target.value)}
                placeholder="POLICY.VERSION.CREATE"
              />
            </div>
            <div>
              <div className="text-xs text-muted-foreground">Resource type</div>
              <Input
                value={resourceTypeFilterInput}
                onChange={(event) => setResourceTypeFilterInput(event.target.value)}
                placeholder="POLICY.VERSION"
              />
            </div>
            <div>
              <div className="text-xs text-muted-foreground">Actor ID</div>
              <Input
                value={actorIDFilterInput}
                onChange={(event) => setActorIDFilterInput(event.target.value)}
                placeholder="admin_demo"
              />
            </div>
            <div className="flex flex-wrap items-end justify-start gap-2 md:justify-end">
              <Select
                value={String(limit)}
                onValueChange={(value) => {
                  setOffset(0);
                  setLimit(Number(value));
                }}
              >
                <SelectTrigger className="w-full md:w-28">
                  <SelectValue placeholder="Limit" />
                </SelectTrigger>
                <SelectContent>
                  {[10, 25, 50, 100].map((value) => (
                    <SelectItem key={value} value={String(value)}>
                      {value} rows
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setOffset(0);
                  setActionFilter(actionFilterInput);
                  setResourceTypeFilter(resourceTypeFilterInput);
                  setActorIDFilter(actorIDFilterInput);
                }}
                disabled={!filtersActive}
                className="w-full md:w-auto"
              >
                Apply
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setActionFilterInput("");
                  setResourceTypeFilterInput("");
                  setActorIDFilterInput("");
                  setActionFilter("");
                  setResourceTypeFilter("");
                  setActorIDFilter("");
                  setOffset(0);
                }}
                disabled={!filtersActive}
                className="w-full md:w-auto"
              >
                Clear
              </Button>
            </div>
          </div>
          <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <div className="text-xs text-muted-foreground">Page {pageNumber}</div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setOffset(offset - limit)}
                disabled={!canPrev}
                className="flex-1 md:flex-none"
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setOffset(offset + limit)}
                disabled={!canNext}
                className="flex-1 md:flex-none"
              >
                Next
              </Button>
              <Button variant="outline" size="sm" onClick={() => refresh(offset)} className="flex-1 md:flex-none">
                Refresh
              </Button>
            </div>
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
                    <TableCell>{event.actor_display ?? event.actor_id ?? "—"}</TableCell>
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
