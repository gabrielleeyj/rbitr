import { useEffect, useMemo, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { EvidenceRecord, getActionTypes, listEvidence } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { toast } from "sonner";
import { scopeAuditRead, scopePoliciesRead } from "@/lib/scopes";

const decisions = ["ALL", "ALLOW", "DENY", "REQUIRE_APPROVAL"];
const risks = ["ALL", "LOW", "MEDIUM", "HIGH", "CRITICAL"];

export function EvidencePage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const [decisionFilter, setDecisionFilter] = useState<string>("ALL");
  const [riskFilter, setRiskFilter] = useState<string>("ALL");
  const [actionType, setActionType] = useState("ALL");
  const [since, setSince] = useState("");
  const [records, setRecords] = useState<EvidenceRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [actionTypes, setActionTypes] = useState<string[]>([]);
  const canReadEvidence = hasScope(scopeAuditRead);
  const canReadActionTypes = hasScope(scopePoliciesRead);

  const canQuery = useMemo(
    () => Boolean(adminKey && selectedTenant && canReadEvidence),
    [adminKey, selectedTenant, canReadEvidence]
  );

  const loadEvidence = async () => {
    if (!adminKey || !selectedTenant) {
      return;
    }
    setLoading(true);
    setError("");
    try {
      const data = await listEvidence(
        { adminKey },
        selectedTenant.tenant_id,
        {
          limit: 50,
          decision: decisionFilter !== "ALL" ? decisionFilter : undefined,
          action_type: actionType && actionType !== "ALL" ? actionType : undefined,
          risk: riskFilter !== "ALL" ? riskFilter : undefined,
          since: since || undefined,
        }
      );
      setRecords(data.records ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load evidence.");
    } finally {
      setLoading(false);
    }
  };

  const downloadEvidence = async () => {
    if (!adminKey || !selectedTenant) {
      return;
    }
    try {
      const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "";
      const response = await fetch(
        `${baseUrl}/admin/tenants/${selectedTenant.tenant_id}/evidence?limit=50`,
        {
          headers: {
            Authorization: `Bearer ${adminKey}`,
          },
        }
      );
      if (!response.ok) {
        throw new Error(`Download failed: ${response.status}`);
      }
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `evidence_${selectedTenant.tenant_id}.json`;
      anchor.click();
      window.URL.revokeObjectURL(url);
      toast.success("Evidence downloaded");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to download evidence.");
    }
  };

  useEffect(() => {
    if (canQuery) {
      loadEvidence();
    } else {
      setRecords([]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canQuery]);

  useEffect(() => {
    let mounted = true;
    const loadActionTypes = async () => {
      if (!adminKey || !canReadActionTypes) {
        if (mounted) {
          setActionTypes([]);
        }
        return;
      }
      try {
        const data = await getActionTypes({ adminKey });
        if (!mounted) return;
        setActionTypes(data.action_types ?? []);
      } catch {
        if (!mounted) return;
        setActionTypes([]);
      }
    };
    loadActionTypes();
    return () => {
      mounted = false;
    };
  }, [adminKey, canReadActionTypes]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Evidence</CardTitle>
        <CardDescription>Review governed actions and export evidence packs.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {!selectedTenant ? (
          <Alert>
            <AlertDescription>Select a tenant to view evidence.</AlertDescription>
          </Alert>
        ) : null}
        {!canReadEvidence ? (
          <Alert>
            <AlertDescription>Missing scope: {scopeAuditRead}</AlertDescription>
          </Alert>
        ) : null}
        <div className="grid gap-4 md:grid-cols-4">
          <div className="space-y-2">
            <Label>Decision</Label>
            <Select value={decisionFilter} onValueChange={setDecisionFilter}>
              <SelectTrigger>
                <SelectValue placeholder="All decisions" />
              </SelectTrigger>
              <SelectContent>
                {decisions.map((decision) => (
                  <SelectItem key={decision} value={decision}>
                    {decision}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Risk</Label>
            <Select value={riskFilter} onValueChange={setRiskFilter}>
              <SelectTrigger>
                <SelectValue placeholder="All risks" />
              </SelectTrigger>
              <SelectContent>
                {risks.map((risk) => (
                  <SelectItem key={risk} value={risk}>
                    {risk}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Action type</Label>
            {actionTypes.length > 0 ? (
              <Select value={actionType} onValueChange={setActionType}>
                <SelectTrigger>
                  <SelectValue placeholder="All action types" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ALL">All</SelectItem>
                  {actionTypes.map((type) => (
                    <SelectItem key={type} value={type}>
                      {type}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                value={actionType === "ALL" ? "" : actionType}
                onChange={(event) => setActionType(event.target.value)}
                placeholder="DATA.EXPORT"
              />
            )}
          </div>
          <div className="space-y-2">
            <Label>Since (ISO)</Label>
            <Input value={since} onChange={(event) => setSince(event.target.value)} placeholder="2026-01-01T00:00:00Z" />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button onClick={loadEvidence} disabled={!canQuery || loading}>
            {loading ? "Loading..." : "Refresh"}
          </Button>
          <Button variant="outline" disabled={!canQuery} onClick={downloadEvidence}>
            Download JSON
          </Button>
        </div>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Decision</TableHead>
              <TableHead>Risk</TableHead>
              <TableHead>Tool</TableHead>
              <TableHead>Rule</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground">
                  No evidence records yet. Records appear here once agents make
                  tool calls through the gateway for this tenant.
                </TableCell>
              </TableRow>
            ) : (
              records.map((record) => (
                <TableRow key={record.decision_id}>
                  <TableCell>{record.timestamp}</TableCell>
                  <TableCell>{record.action_type}</TableCell>
                  <TableCell>{record.decision}</TableCell>
                  <TableCell>{record.action_risk}</TableCell>
                  <TableCell>{record.tool_id}</TableCell>
                  <TableCell>{record.rule_id}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
