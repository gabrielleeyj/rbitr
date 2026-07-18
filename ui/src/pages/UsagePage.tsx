import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { BarChart3, AlertTriangle } from "lucide-react";
import { ListSkeleton } from "@/components/list-skeleton";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import { useAdminKey } from "@/lib/auth";
import { scopeSettingsRead } from "@/lib/scopes";
import {
  getUsageSummary,
  getUsageHistory,
  type UsageSummaryResponse,
  type UsageHistoryPeriod,
  type UsageGauge,
} from "@/lib/api";

const WARN_THRESHOLD = 80;
const CRITICAL_THRESHOLD = 95;

function gaugeColor(pct: number): string {
  if (pct >= CRITICAL_THRESHOLD) return "bg-destructive";
  if (pct >= WARN_THRESHOLD) return "bg-warning";
  return "bg-success";
}

function formatLimit(value: number): string {
  if (value < 0) return "Unlimited";
  return value.toLocaleString();
}

export function UsagePage() {
  const { adminKey, hasScope } = useAdminKey();
  const canRead = hasScope(scopeSettingsRead);

  const [summary, setSummary] = useState<UsageSummaryResponse | null>(null);
  const [history, setHistory] = useState<UsageHistoryPeriod[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const loadData = useCallback(async () => {
    if (!adminKey || !canRead) {
      setLoading(false);
      return;
    }
    try {
      const [summaryData, historyData] = await Promise.all([
        getUsageSummary({ adminKey }),
        getUsageHistory({ adminKey }),
      ]);
      setSummary(summaryData);
      setHistory(historyData.periods);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load usage data.");
    } finally {
      setLoading(false);
    }
  }, [adminKey, canRead]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  if (loading) {
    return (
      <div className="p-6">
        <ListSkeleton />
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6 space-y-4">
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
        <Button onClick={() => void loadData()}>Retry</Button>
      </div>
    );
  }

  if (!summary) return null;

  const actions = summary.usage.governed_actions;
  const tenants = summary.usage.tenants;
  const isFreeTier = summary.tier === "free";

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-display text-lg font-semibold tracking-tight">Usage</h2>
          <p className="text-sm text-muted-foreground">
            Current period: {summary.period}
          </p>
        </div>
        <Badge variant={isFreeTier ? "secondary" : "default"}>
          {summary.tier} tier
        </Badge>
      </div>

      <Separator />

      {/* Quota warning banners */}
      {actions.limit > 0 && actions.pct >= CRITICAL_THRESHOLD && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            You have used {actions.pct.toFixed(1)}% of your monthly action quota.
            {isFreeTier && (
              <>
                {" "}
                <Link to="/license" className="underline font-medium">
                  Upload a license key
                </Link>{" "}
                to remove limits.
              </>
            )}
          </AlertDescription>
        </Alert>
      )}
      {actions.limit > 0 &&
        actions.pct >= WARN_THRESHOLD &&
        actions.pct < CRITICAL_THRESHOLD && (
          <Alert>
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              You have used {actions.pct.toFixed(1)}% of your monthly action quota.
              {isFreeTier && (
                <>
                  {" "}
                  <Link to="/license" className="underline font-medium">
                    Upload a license key
                  </Link>{" "}
                  to remove limits.
                </>
              )}
            </AlertDescription>
          </Alert>
        )}

      {/* Resource gauges */}
      <div className="grid gap-4 md:grid-cols-2">
        <GaugeCard
          title="Governed Actions"
          description="Monthly action count"
          gauge={actions}
        />
        <GaugeCard
          title="Tenants"
          description="Active tenants"
          gauge={tenants}
        />
      </div>

      {/* Feature availability */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Feature Availability</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <FeatureRow label="Approval Workflows" enabled={summary.features.approval_workflows} />
            <FeatureRow label="Evidence Export" enabled={summary.features.evidence_export} />
            <FeatureRow label="Integrations" enabled={summary.features.integrations} />
            <FeatureRow label="Custom Policies" enabled={summary.features.custom_policies} />
          </div>
          <div className="mt-3 pt-3 border-t text-sm text-muted-foreground">
            Audit retention: {summary.audit_retention_days} days
          </div>
        </CardContent>
      </Card>

      {/* Usage history */}
      {history.length > 0 && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <BarChart3 className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">Usage History</CardTitle>
            </div>
            <CardDescription>Monthly governed actions</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {history.map((period) => (
                <div key={period.period} className="flex items-center gap-3">
                  <span className="text-sm font-mono w-20 shrink-0">
                    {period.period}
                  </span>
                  <div className="flex-1">
                    <Progress
                      value={Math.min(period.pct, 100)}
                      indicatorClassName={gaugeColor(period.pct)}
                    />
                  </div>
                  <span className="text-sm text-muted-foreground w-24 text-right shrink-0">
                    {period.action_count.toLocaleString()}
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* CTA for free tier */}
      {isFreeTier && (
        <Card className="border-dashed">
          <CardContent className="pt-6 text-center">
            <p className="text-sm text-muted-foreground mb-3">
              Upload a license key to remove usage limits and unlock all features.
            </p>
            <Button asChild variant="outline" size="sm">
              <Link to="/license">Go to License Settings</Link>
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function GaugeCard({
  title,
  description,
  gauge,
}: {
  title: string;
  description: string;
  gauge: UsageGauge;
}) {
  const isUnlimited = gauge.limit < 0;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="flex items-baseline justify-between">
          <span className="text-2xl font-bold">{gauge.used.toLocaleString()}</span>
          <span className="text-sm text-muted-foreground">
            / {formatLimit(gauge.limit)}
          </span>
        </div>
        {!isUnlimited && (
          <Progress
            value={Math.min(gauge.pct, 100)}
            indicatorClassName={gaugeColor(gauge.pct)}
          />
        )}
        {isUnlimited && (
          <div className="text-xs text-muted-foreground">No limit</div>
        )}
      </CardContent>
    </Card>
  );
}

function FeatureRow({ label, enabled }: { label: string; enabled: boolean }) {
  return (
    <div className="flex items-center justify-between">
      <span>{label}</span>
      <Badge variant={enabled ? "default" : "secondary"} className="text-xs">
        {enabled ? "Available" : "Locked"}
      </Badge>
    </div>
  );
}
