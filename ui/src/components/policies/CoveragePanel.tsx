import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getPolicyCoverage,
  type PolicyCoverageGap,
  type PolicyCoverageReport,
  type StructuredRule,
} from "@/lib/api";

interface CoveragePanelProps {
  adminKey: string;
  tenantId: string;
  refreshToken: number;
  onConfigure: (rule: StructuredRule) => void;
}

/** gapToRule converts an ambiguous endpoint into a seed rule for the builder. */
function gapToRule(gap: PolicyCoverageGap): StructuredRule {
  const match: StructuredRule["match"] = { tool_ids: [gap.tool_id] };
  if (gap.action_type) {
    match.action_types = [gap.action_type];
  }
  return {
    id: "rule_new",
    priority: 100,
    effect: "REQUIRE_APPROVAL",
    match,
  };
}

function reasonLabel(gap: PolicyCoverageGap): string {
  if (gap.reason === "no_traffic") {
    return "Never called";
  }
  return gap.current_fallback_decision
    ? `Fallback: ${gap.current_fallback_decision}`
    : "Ambiguous";
}

export function CoveragePanel({ adminKey, tenantId, refreshToken, onConfigure }: CoveragePanelProps) {
  const [report, setReport] = useState<PolicyCoverageReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await getPolicyCoverage({ adminKey }, tenantId);
      setReport(data);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load coverage";
      setError(message);
      toast.error(message);
    } finally {
      setLoading(false);
    }
  }, [adminKey, tenantId]);

  useEffect(() => {
    void load();
  }, [load, refreshToken]);

  const gaps = report?.gaps ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Endpoints needing configuration</CardTitle>
        <CardDescription>
          Endpoints governed only by a catch-all fallback, or never exercised. Configure one to
          add an explicit rule.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading coverage…</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : gaps.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No ambiguous endpoints — every registered tool and observed action is covered.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Tool</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Hits</TableHead>
                <TableHead className="text-right">Configure</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {gaps.map((gap) => (
                <TableRow key={`${gap.tool_id}:${gap.action_type ?? ""}:${gap.reason}`}>
                  <TableCell className="font-mono text-xs">{gap.tool_id}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {gap.action_type ?? <span className="text-muted-foreground">—</span>}
                  </TableCell>
                  <TableCell>
                    <Badge variant={gap.reason === "no_traffic" ? "outline" : "secondary"}>
                      {reasonLabel(gap)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right text-xs">{gap.hit_count}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => onConfigure(gapToRule(gap))}
                    >
                      Set permission
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
