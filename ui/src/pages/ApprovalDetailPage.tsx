import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Textarea } from "@/components/ui/textarea";
import { Separator } from "@/components/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { approveApproval, denyApproval, getApproval, revokeApproval } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import type { ApprovalRequest } from "@/lib/api";
import { toast } from "sonner";
import { scopeApprovalsDecide, scopeApprovalsRead } from "@/lib/scopes";

export function ApprovalDetailPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const { approvalId } = useParams();
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [action, setAction] = useState<"approve" | "deny" | "revoke" | null>(null);
  const [comment, setComment] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const canReadApprovals = hasScope(scopeApprovalsRead);
  const canDecideApprovals = hasScope(scopeApprovalsDecide);

  const dialogTitle = useMemo(() => {
    if (!action) return "";
    if (action === "approve") return "Approve request";
    if (action === "deny") return "Deny request";
    return "Revoke approval";
  }, [action]);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !tenantId || !approvalId || !canReadApprovals) {
        setApproval(null);
        setLoading(false);
        return;
      }
      try {
        setLoading(true);
        const data = await getApproval({ adminKey }, tenantId, approvalId);
        if (!mounted) return;
        setApproval(data);
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load approval.");
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId, approvalId, canReadApprovals]);

  const openDialog = (nextAction: "approve" | "deny" | "revoke") => {
    if (!canDecideApprovals) {
      return;
    }
    setAction(nextAction);
    setComment("");
    setDialogOpen(true);
  };

  const handleDecision = async () => {
    if (!adminKey || !tenantId || !approval || !action || !canDecideApprovals) return;
    try {
      setSubmitting(true);
      if (action === "approve") {
        const updated = await approveApproval({ adminKey }, tenantId, approval.approval_request_id, comment);
        setApproval(updated);
        toast.success("Approval granted");
      } else if (action === "deny") {
        const updated = await denyApproval({ adminKey }, tenantId, approval.approval_request_id, comment);
        setApproval(updated);
        toast.success("Request denied");
      } else {
        const updated = await revokeApproval({ adminKey }, tenantId, approval.approval_request_id, comment);
        setApproval(updated);
        toast.success("Approval revoked");
      }
      setDialogOpen(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed");
    } finally {
      setSubmitting(false);
    }
  };

  const formatDate = (value?: string) => value ?? "—";

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Approval request</h2>
          <p className="text-sm text-muted-foreground">Request details and execution metadata.</p>
        </div>
        <Button variant="outline" asChild>
          <Link to="/approvals">Back to approvals</Link>
        </Button>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {!canReadApprovals ? (
        <Alert>
          <AlertDescription>Missing scope: {scopeApprovalsRead}</AlertDescription>
        </Alert>
      ) : null}

      {loading ? (
        <div className="text-sm text-muted-foreground">Loading approval...</div>
      ) : approval ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-sm">{approval.approval_request_id}</span>
              <Badge variant="secondary">{approval.status}</Badge>
            </CardTitle>
            <CardDescription>{approval.action_summary || approval.action_type}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 md:grid-cols-3 text-sm">
              <div>
                <div className="text-xs text-muted-foreground">Action</div>
                <div>{approval.action_type}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Risk</div>
                <div>{approval.risk ?? "—"}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Rule</div>
                <div>{approval.rule_id ?? "—"}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Policy version</div>
                <div>{approval.policy_version ?? "—"}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Expires at</div>
                <div>{formatDate(approval.expires_at)}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Created at</div>
                <div>{formatDate(approval.created_at)}</div>
              </div>
            </div>

            <Separator />

            <div className="grid gap-3 md:grid-cols-2 text-sm">
              <div>
                <div className="text-xs text-muted-foreground">Decided by</div>
                <div>{approval.decided_by ?? "—"}</div>
                <div className="text-xs text-muted-foreground">{formatDate(approval.decided_at)}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Decision comment</div>
                <div>{approval.decision_comment || "—"}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Executed request</div>
                <div className="font-mono text-xs">{approval.executed_request_id ?? "—"}</div>
                <div className="text-xs text-muted-foreground">{formatDate(approval.executed_at)}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">Executed decision</div>
                <div className="font-mono text-xs">{approval.executed_decision_id ?? "—"}</div>
              </div>
            </div>

            <Separator />

            <div className="text-sm">
              <div className="text-xs text-muted-foreground">Request hash</div>
              <div className="font-mono text-xs break-all">{approval.request_hash}</div>
            </div>

            {approval.request_context ? (
              <div className="text-sm">
                <div className="text-xs text-muted-foreground">Request context</div>
                <pre className="mt-1 max-h-72 overflow-auto rounded border border-border/60 bg-muted/30 p-3 font-mono text-xs leading-relaxed">
                  {JSON.stringify(approval.request_context, null, 2)}
                </pre>
              </div>
            ) : null}

            {approval.reasons && approval.reasons.length > 0 ? (
              <div className="text-sm">
                <div className="text-xs text-muted-foreground">Reasons</div>
                <ul className="list-disc pl-5">
                  {approval.reasons.map((reason) => (
                    <li key={reason.code}>
                      <span className="font-medium">{reason.code}</span>: {reason.message}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}

            <div className="flex flex-wrap gap-2">
              {approval.status === "PENDING" && canDecideApprovals ? (
                <>
                  <Button size="sm" onClick={() => openDialog("approve")}>
                    Approve
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => openDialog("deny")}>
                    Deny
                  </Button>
                </>
              ) : null}
              {approval.status === "APPROVED" && canDecideApprovals ? (
                <Button size="sm" variant="outline" onClick={() => openDialog("revoke")}>
                  Revoke
                </Button>
              ) : null}
            </div>
          </CardContent>
        </Card>
      ) : null}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialogTitle}</DialogTitle>
            <DialogDescription>
              Add an optional note to explain this decision. The decision is written to the audit log.
            </DialogDescription>
          </DialogHeader>
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
            <Button onClick={handleDecision} disabled={submitting || !canDecideApprovals}>
              {submitting ? "Saving..." : "Confirm"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
