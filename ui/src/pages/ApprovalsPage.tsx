import { useEffect, useMemo, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { approveApproval, denyApproval, listApprovals, revokeApproval } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import type { ApprovalRequest } from "@/lib/api";
import { toast } from "sonner";
import { Link } from "react-router-dom";

const statusTabs = ["PENDING", "APPROVED", "EXECUTING", "EXECUTED", "FAILED", "DENIED", "EXPIRED"];

export function ApprovalsPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [approvals, setApprovals] = useState<ApprovalRequest[]>([]);
  const [status, setStatus] = useState("PENDING");
  const [limit, setLimit] = useState(10);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [action, setAction] = useState<"approve" | "deny" | "revoke" | null>(null);
  const [comment, setComment] = useState("");
  const [activeApproval, setActiveApproval] = useState<ApprovalRequest | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const pageNumber = Math.floor(offset / limit) + 1;
  const canPrev = offset > 0;
  const canNext = hasMore;

  const refresh = async (nextOffset = offset) => {
    if (!adminKey || !tenantId) return;
    const data = await listApprovals(
      { adminKey },
      tenantId,
      { status, limit: limit + 1, offset: nextOffset }
    );
    setHasMore(data.length > limit);
    setApprovals(data.slice(0, limit));
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
        setError("");
        const data = await listApprovals(
          { adminKey },
          tenantId,
          { status, limit: limit + 1, offset }
        );
        if (!mounted) return;
        setHasMore(data.length > limit);
        setApprovals(data.slice(0, limit));
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load approvals.");
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId, status, limit, offset]);

  const dialogTitle = useMemo(() => {
    if (!action) return "";
    if (action === "approve") return "Approve request";
    if (action === "deny") return "Deny request";
    return "Revoke approval";
  }, [action]);

  const openDialog = (approval: ApprovalRequest, nextAction: "approve" | "deny" | "revoke") => {
    setActiveApproval(approval);
    setAction(nextAction);
    setComment("");
    setDialogOpen(true);
  };

  const handleDecision = async () => {
    if (!adminKey || !tenantId || !activeApproval || !action) return;
    try {
      setSubmitting(true);
      if (action === "approve") {
        await approveApproval({ adminKey }, tenantId, activeApproval.approval_request_id, comment);
        toast.success("Approval granted");
      } else if (action === "deny") {
        await denyApproval({ adminKey }, tenantId, activeApproval.approval_request_id, comment);
        toast.success("Request denied");
      } else {
        await revokeApproval({ adminKey }, tenantId, activeApproval.approval_request_id, comment);
        toast.success("Approval revoked");
      }
      setDialogOpen(false);
      await refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed");
    } finally {
      setSubmitting(false);
    }
  };

  const formatDate = (value?: string) => {
    if (!value) return "—";
    return value;
  };

  const contextSummary = (approval: ApprovalRequest) => {
    const context = approval.request_context;
    if (!context) return "";
    const method = typeof context.http_method === "string" ? context.http_method : "";
    const path = typeof context.path === "string" ? context.path : "";
    if (method || path) {
      return `${method} ${path}`.trim();
    }
    const mcpMethod = typeof context.method === "string" ? context.method : "";
    const toolName = typeof context.tool_name === "string" ? context.tool_name : "";
    if (mcpMethod || toolName) {
      return `${mcpMethod} ${toolName}`.trim();
    }
    return "";
  };

  const selectedStatusLabel = status.toLowerCase().replace("_", " ");

  return (
    <Card>
      <CardHeader>
        <CardTitle>Approvals</CardTitle>
        <CardDescription>Resolve pending approvals and review execution history.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!tenantId ? (
          <Alert>
            <AlertDescription>Select a tenant to view approvals.</AlertDescription>
          </Alert>
        ) : null}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        <div className="space-y-3">
          <Tabs value={status} onValueChange={(value) => {
            setStatus(value);
            setOffset(0);
          }}>
            <TabsList className="flex flex-wrap gap-2">
              {statusTabs.map((value) => (
                <TabsTrigger key={value} value={value}>
                  {value}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>

          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="flex items-center gap-3">
              <div className="text-xs text-muted-foreground">Status</div>
              <Badge variant="secondary">{selectedStatusLabel}</Badge>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">Rows</span>
                <Input
                  type="number"
                  value={limit}
                  min={1}
                  max={50}
                  onChange={(event) => setLimit(Number(event.target.value) || 10)}
                  className="h-8 w-20"
                />
              </div>
              <Button
                variant="outline"
                size="sm"
                disabled={!canPrev}
                onClick={() => setOffset(Math.max(0, offset - limit))}
              >
                Prev
              </Button>
              <div className="text-xs text-muted-foreground">Page {pageNumber}</div>
              <Button
                variant="outline"
                size="sm"
                disabled={!canNext}
                onClick={() => setOffset(offset + limit)}
              >
                Next
              </Button>
            </div>
          </div>
        </div>

        {!tenantId ? null : loading ? (
          <div className="text-sm text-muted-foreground">Loading approvals...</div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Risk</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Decided</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {approvals.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-sm text-muted-foreground">
                    No approvals in {selectedStatusLabel}.
                  </TableCell>
                </TableRow>
              ) : (
                approvals.map((approval) => (
                  <TableRow key={approval.approval_request_id}>
                    <TableCell className="font-mono text-xs">
                      <Link to={`/approvals/${approval.approval_request_id}`} className="underline-offset-4 hover:underline">
                        {approval.approval_request_id}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm font-medium">{approval.action_type}</div>
                      <div className="text-xs text-muted-foreground">
                        {approval.action_summary || approval.tool_id}
                      </div>
                      {contextSummary(approval) ? (
                        <div className="text-xs text-muted-foreground mt-1 font-mono">
                          {contextSummary(approval)}
                        </div>
                      ) : null}
                    </TableCell>
                    <TableCell>{approval.risk ?? "—"}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{approval.status}</Badge>
                    </TableCell>
                    <TableCell className="text-xs">{formatDate(approval.expires_at)}</TableCell>
                    <TableCell className="text-xs">
                      {approval.decided_by ? (
                        <>
                          <div>{approval.decided_by}</div>
                          <div className="text-muted-foreground">{formatDate(approval.decided_at)}</div>
                        </>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-2">
                        <Button size="sm" variant="ghost" asChild>
                          <Link to={`/approvals/${approval.approval_request_id}`}>View</Link>
                        </Button>
                        {approval.status === "PENDING" ? (
                          <>
                            <Button size="sm" onClick={() => openDialog(approval, "approve")}>
                              Approve
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => openDialog(approval, "deny")}
                            >
                              Deny
                            </Button>
                          </>
                        ) : null}
                        {approval.status === "APPROVED" ? (
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => openDialog(approval, "revoke")}
                          >
                            Revoke
                          </Button>
                        ) : null}
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialogTitle}</DialogTitle>
            <DialogDescription>
              Add an optional note to explain this decision. The decision is written to the audit log.
            </DialogDescription>
          </DialogHeader>
          {activeApproval ? (
            <div className="rounded-lg border border-border/60 bg-muted/40 p-3 text-xs">
              <div className="flex flex-wrap gap-3 text-muted-foreground">
                <span>Request: {activeApproval.approval_request_id}</span>
                <span>Action: {activeApproval.action_type}</span>
                <span>Risk: {activeApproval.risk ?? "—"}</span>
                <span>Rule: {activeApproval.rule_id ?? "—"}</span>
              </div>
              {activeApproval.action_summary ? (
                <div className="mt-2 text-sm text-foreground">{activeApproval.action_summary}</div>
              ) : null}
              {activeApproval.request_context ? (
                <pre className="mt-2 max-h-44 overflow-auto rounded bg-background p-2 font-mono text-[11px] leading-relaxed text-foreground">
                  {JSON.stringify(activeApproval.request_context, null, 2)}
                </pre>
              ) : null}
            </div>
          ) : null}
          <Textarea
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            placeholder="Add a comment (optional)"
            rows={4}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleDecision} disabled={submitting}>
              {submitting ? "Saving..." : "Confirm"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
