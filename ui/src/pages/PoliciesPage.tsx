import { useEffect, useMemo, useRef, useState } from "react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { toast } from "sonner";
import {
  createPolicyVersion,
  getActionTypes,
  getPolicyVersion,
  getStructuredPolicy,
  listPolicies,
  listTools,
  publishPolicyVersion,
  rollbackPolicyVersion,
  simulatePolicy,
  type PolicySimulationResponse,
  type PolicyVersion,
  type StructuredPolicy,
  type StructuredRule,
} from "@/lib/api";
import { RuleBuilder, emptyStructuredPolicy } from "@/components/policies/RuleBuilder";
import { CoveragePanel } from "@/components/policies/CoveragePanel";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import {
  scopePoliciesPublish,
  scopePoliciesRead,
  scopePoliciesRollback,
  scopePoliciesSimulate,
  scopePoliciesWrite,
} from "@/lib/scopes";

export function PoliciesPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const [versions, setVersions] = useState<PolicyVersion[]>([]);
  const [activeVersion, setActiveVersion] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [policyVersion, setPolicyVersion] = useState("");
  const [policyNotes, setPolicyNotes] = useState("");
  const [regoModule, setRegoModule] = useState("");
  const [rollbackVersion, setRollbackVersion] = useState("");
  const [simulateInput, setSimulateInput] = useState('{"tenant_id":"t_demo"}');
  const [simulateResult, setSimulateResult] =
    useState<PolicySimulationResponse | null>(null);
  const [baseRego, setBaseRego] = useState("");
  const [compileStatus, setCompileStatus] = useState<"idle" | "ok" | "error">(
    "idle",
  );
  const [simulateStatus, setSimulateStatus] = useState<"idle" | "ok" | "error">(
    "idle",
  );
  const [diffPreview, setDiffPreview] = useState("");
  const [viewMode, setViewMode] = useState<"builder" | "advanced">("builder");
  const [actionTypesList, setActionTypesList] = useState<string[]>([]);
  const [toolsList, setToolsList] = useState<string[]>([]);
  const [structuredInitial, setStructuredInitial] = useState<StructuredPolicy | null>(null);
  const [advancedActive, setAdvancedActive] = useState(false);
  const [seedRule, setSeedRule] = useState<StructuredRule | null>(null);
  const [coverageRefresh, setCoverageRefresh] = useState(0);
  const lastLoadedKeyRef = useRef<string | null>(null);
  const refreshInFlight = useRef<Promise<void> | null>(null);
  const canReadPolicies = hasScope(scopePoliciesRead);
  const canWritePolicies = hasScope(scopePoliciesWrite);
  const canPublishPolicies = hasScope(scopePoliciesPublish);
  const canRollbackPolicies = hasScope(scopePoliciesRollback);
  const canSimulatePolicies = hasScope(scopePoliciesSimulate);

  const tenantId = selectedTenant?.tenant_id;
  const sortedVersions = useMemo(
    () =>
      versions
        .slice()
        .sort((a, b) => b.policy_version.localeCompare(a.policy_version)),
    [versions],
  );

  const publishReady =
    compileStatus === "ok" && simulateStatus === "ok" && diffPreview !== "";
  const simulationSummary = useMemo(() => {
    if (!simulateResult) {
      return null;
    }
    const topMatchedRule = simulateResult.decision.matched_rules?.[0];
    const effectiveRule = topMatchedRule
      ? {
          id: topMatchedRule.rule_id,
          priority: topMatchedRule.priority,
          effect: topMatchedRule.effect,
          reasons:
            topMatchedRule.reasons && topMatchedRule.reasons.length > 0
              ? topMatchedRule.reasons
              : simulateResult.decision.reasons,
        }
      : {
          id: simulateResult.decision.rule.id,
          priority: simulateResult.decision.rule.priority,
          effect: simulateResult.decision.decision,
          reasons: simulateResult.decision.reasons,
        };
    return {
      decision: simulateResult.decision.decision,
      risk: simulateResult.decision.risk,
      rule: effectiveRule,
    };
  }, [simulateResult]);

  const generateVersion = () => {
    const now = new Date();
    const pad = (value: number) => value.toString().padStart(2, "0");
    return `p_${now.getUTCFullYear()}_${pad(now.getUTCMonth() + 1)}_${pad(now.getUTCDate())}_${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}`;
  };

  const computeDiffPreview = (base: string, updated: string) => {
    if (!base && !updated) {
      return "";
    }
    if (base === updated) {
      return "";
    }
    const baseLines = base.split("\n");
    const updatedLines = updated.split("\n");
    const maxLines = Math.max(baseLines.length, updatedLines.length);
    const preview: string[] = [];
    for (let i = 0; i < maxLines; i += 1) {
      const oldLine = baseLines[i];
      const newLine = updatedLines[i];
      if (oldLine === newLine) {
        preview.push(`  ${oldLine ?? ""}`);
      } else {
        if (oldLine !== undefined) {
          preview.push(`- ${oldLine}`);
        }
        if (newLine !== undefined) {
          preview.push(`+ ${newLine}`);
        }
      }
    }
    return preview.join("\n");
  };

  const normalizeMultilineStrings = (value: string) => {
    if (!value) {
      return value;
    }
    let output = "";
    let inString = false;
    let escaped = false;
    for (let i = 0; i < value.length; i += 1) {
      const ch = value[i];
      if (ch === "\n" && inString) {
        output += "\\n";
        continue;
      }
      output += ch;
      if (escaped) {
        escaped = false;
        continue;
      }
      if (ch === "\\") {
        escaped = true;
        continue;
      }
      if (ch === '"') {
        inString = !inString;
      }
    }
    return output;
  };

  const findMultilineStrings = (value: string) => {
    const issues: Array<{ line: number }> = [];
    let inString = false;
    let escaped = false;
    let line = 1;
    for (let i = 0; i < value.length; i += 1) {
      const ch = value[i];
      if (ch === "\n") {
        if (inString) {
          issues.push({ line });
        }
        line += 1;
        escaped = false;
        continue;
      }
      if (escaped) {
        escaped = false;
        continue;
      }
      if (ch === "\\") {
        escaped = true;
        continue;
      }
      if (ch === '"') {
        inString = !inString;
      }
    }
    return issues;
  };

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !tenantId || !canReadPolicies) {
        setVersions([]);
        setActiveVersion("");
        setLoading(false);
        return;
      }
      const loadKey = `${adminKey}:${tenantId}`;
      if (lastLoadedKeyRef.current === loadKey) {
        return;
      }
      try {
        setLoading(true);
        const data = await listPolicies({ adminKey }, tenantId);
        if (!mounted) return;
        setVersions(data.versions ?? []);
        setActiveVersion(data.active_policy_version ?? "");
        if (data.active_policy_version) {
          try {
            const active = await getPolicyVersion(
              { adminKey },
              tenantId,
              data.active_policy_version,
            );
            if (!mounted) return;
            setBaseRego(active.rego_module);
            setRegoModule(active.rego_module);
          } catch (err) {
            if (!mounted) return;
            setActionError(
              err instanceof Error
                ? err.message
                : "Failed to load active policy module.",
            );
          }
        }
        setPolicyVersion(generateVersion());
        setLoading(false);
        lastLoadedKeyRef.current = loadKey;
      } catch (err) {
        if (!mounted) return;
        setError(
          err instanceof Error
            ? err.message
            : "Failed to load policy versions.",
        );
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId, canReadPolicies]);

  // Load builder inputs (action types, tools) and the structured form of the
  // active policy version so the rule builder can round-trip it.
  useEffect(() => {
    let mounted = true;
    const loadBuilderData = async () => {
      if (!adminKey || !tenantId || !canReadPolicies) {
        return;
      }
      try {
        const [actionTypes, tools] = await Promise.all([
          getActionTypes({ adminKey }),
          listTools({ adminKey }, tenantId),
        ]);
        if (!mounted) return;
        setActionTypesList(actionTypes.action_types ?? []);
        setToolsList((tools ?? []).map((tool) => tool.tool_id));
      } catch {
        // Builder inputs are non-critical; leave selectors empty on failure.
      }

      if (!activeVersion) {
        setStructuredInitial(emptyStructuredPolicy);
        setAdvancedActive(false);
        return;
      }
      try {
        const structured = await getStructuredPolicy({ adminKey }, tenantId, activeVersion);
        if (!mounted) return;
        setAdvancedActive(structured.advanced_mode);
        setStructuredInitial(structured.structured ?? emptyStructuredPolicy);
      } catch {
        if (!mounted) return;
        setStructuredInitial(emptyStructuredPolicy);
        setAdvancedActive(false);
      }
    };
    void loadBuilderData();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId, activeVersion, canReadPolicies, coverageRefresh]);

  const handleStructuredSaved = () => {
    setCoverageRefresh((value) => value + 1);
    void refresh();
  };

  const handleConfigureGap = (rule: StructuredRule) => {
    setViewMode("builder");
    setSeedRule(rule);
  };

  useEffect(() => {
    setDiffPreview(computeDiffPreview(baseRego, regoModule));
  }, [baseRego, regoModule]);

  useEffect(() => {
    setCompileStatus("idle");
    setSimulateStatus("idle");
    setSimulateResult(null);
  }, [regoModule]);

  const refresh = async () => {
    if (!adminKey || !tenantId || !canReadPolicies) return;
    if (refreshInFlight.current) {
      await refreshInFlight.current;
      return;
    }
    setLoading(true);
    setError("");
    refreshInFlight.current = (async () => {
      try {
        const data = await listPolicies({ adminKey }, tenantId);
        setVersions(data.versions ?? []);
        setActiveVersion(data.active_policy_version ?? "");
        lastLoadedKeyRef.current = `${adminKey}:${tenantId}`;
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Failed to refresh policies.",
        );
      } finally {
        setLoading(false);
      }
    })();
    await refreshInFlight.current;
    refreshInFlight.current = null;
  };

  const handlePublish = async (version: string) => {
    if (!adminKey || !tenantId || !canPublishPolicies) return;
    setActionError("");
    try {
      await publishPolicyVersion({ adminKey }, tenantId, version);
      await refresh();
      toast.success("Policy published", {
        description: `Active version: ${version}`,
      });
    } catch (err) {
      setActionError(
        err instanceof Error ? err.message : "Failed to publish policy.",
      );
    }
  };

  const handleRollback = async () => {
    if (!adminKey || !tenantId || !canRollbackPolicies) return;
    setActionError("");
    try {
      await rollbackPolicyVersion(
        { adminKey },
        tenantId,
        rollbackVersion || undefined,
      );
      await refresh();
      toast.success("Policy rolled back");
    } catch (err) {
      setActionError(
        err instanceof Error ? err.message : "Failed to rollback policy.",
      );
    }
  };

  const handleCreate = async () => {
    if (!adminKey || !tenantId || !canWritePolicies) return;
    setActionError("");
    const normalized = normalizeMultilineStrings(regoModule);
    try {
      await createPolicyVersion({ adminKey }, tenantId, {
        policy_version: policyVersion,
        rego_module: normalized,
        notes: policyNotes,
      });
      setPolicyVersion(generateVersion());
      setPolicyNotes("");
      await refresh();
      toast.success("Policy version created", { description: policyVersion });
    } catch (err) {
      setActionError(
        err instanceof Error ? err.message : "Failed to create policy version.",
      );
    }
  };

  const handleCreateAndPublish = async () => {
    if (!adminKey || !tenantId || !canWritePolicies || !canPublishPolicies) return;
    setActionError("");
    const normalized = normalizeMultilineStrings(regoModule);
    try {
      await createPolicyVersion({ adminKey }, tenantId, {
        policy_version: policyVersion,
        rego_module: normalized,
        notes: policyNotes,
      });
      await publishPolicyVersion({ adminKey }, tenantId, policyVersion);
      setBaseRego(normalized);
      setPolicyVersion(generateVersion());
      setPolicyNotes("");
      setCompileStatus("idle");
      setSimulateStatus("idle");
      setSimulateResult(null);
      toast.success("Policy published", { description: policyVersion });
    } catch (err) {
      setActionError(
        err instanceof Error
          ? err.message
          : "Failed to publish policy version.",
      );
    } finally {
      await refresh();
    }
  };

  const handleCompileCheck = async () => {
    if (!adminKey || !tenantId || !canSimulatePolicies) return;
    const lintIssues = findMultilineStrings(regoModule);
    if (lintIssues.length > 0) {
      setCompileStatus("error");
      setActionError(
        `Invalid string literal detected on line ${lintIssues[0].line}. Use \\n to encode new lines in quoted strings.`,
      );
      return;
    }
    setActionError("");
    setCompileStatus("idle");
    try {
      const normalized = normalizeMultilineStrings(regoModule);
      await simulatePolicy({ adminKey }, tenantId, {
        rego_module: normalized,
        input: {},
      });
      setCompileStatus("ok");
      toast.success("Policy compiled");
    } catch (err) {
      setCompileStatus("error");
      setActionError(err instanceof Error ? err.message : "Compile failed.");
    }
  };

  const handleSimulate = async () => {
    if (!adminKey || !tenantId || !canSimulatePolicies) return;
    const lintIssues = findMultilineStrings(regoModule);
    if (lintIssues.length > 0) {
      setSimulateStatus("error");
      setActionError(
        `Invalid string literal detected on line ${lintIssues[0].line}. Use \\n to encode new lines in quoted strings.`,
      );
      return;
    }
    setActionError("");
    setSimulateResult(null);
    setSimulateStatus("idle");
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(simulateInput || "{}");
    } catch (err) {
      setActionError(
        err instanceof Error
          ? err.message
          : "Simulation input must be valid JSON.",
      );
      return;
    }
    try {
      const normalized = normalizeMultilineStrings(regoModule);
      const response = await simulatePolicy(
        { adminKey },
        tenantId,
        normalized
          ? { rego_module: normalized, input: parsed }
          : { input: parsed },
      );
      setSimulateResult(response);
      setSimulateStatus("ok");
      toast.success("Simulation complete");
    } catch (err) {
      setSimulateStatus("error");
      setActionError(
        err instanceof Error ? err.message : "Failed to simulate policy.",
      );
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Policy versions</CardTitle>
          <CardDescription>
            Review active policy versions and manage rollbacks.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {!canReadPolicies ? (
            <Alert>
              <AlertDescription>Missing scope: {scopePoliciesRead}</AlertDescription>
            </Alert>
          ) : null}
          {!tenantId ? (
            <div className="text-sm text-muted-foreground">
              Select a tenant to view policies.
            </div>
          ) : loading ? (
            <div className="text-sm text-muted-foreground">
              Loading policy versions...
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex justify-end">
                <Button variant="outline" size="sm" onClick={refresh} disabled={!canReadPolicies}>
                  Refresh
                </Button>
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Version</TableHead>
                    <TableHead>Notes</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedVersions.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-muted-foreground">
                        No policy versions yet.
                      </TableCell>
                    </TableRow>
                  ) : (
                    sortedVersions.map((version) => (
                      <TableRow key={version.policy_version}>
                        <TableCell className="font-medium">
                          {version.policy_version}
                        </TableCell>
                        <TableCell>{version.notes || "—"}</TableCell>
                        <TableCell>
                          {version.created_at
                            ? new Date(version.created_at).toLocaleString()
                            : "—"}
                        </TableCell>
                        <TableCell>
                          {activeVersion === version.policy_version ? (
                            <Badge variant="secondary">Active</Badge>
                          ) : (
                            <Badge variant="outline">Inactive</Badge>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={activeVersion === version.policy_version || !canPublishPolicies}
                            onClick={() =>
                              handlePublish(version.policy_version)
                            }
                          >
                            Publish
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          )}
          <div className="grid gap-4 md:grid-cols-[1fr_200px]">
            <div className="space-y-2">
              <Label htmlFor="rollback-version">
                Rollback to version (optional)
              </Label>
              <Input
                id="rollback-version"
                value={rollbackVersion}
                onChange={(event) => setRollbackVersion(event.target.value)}
                placeholder="Leave empty to rollback to previous"
                disabled={!canRollbackPolicies}
              />
            </div>
            <div className="flex items-end">
              <Button
                variant="outline"
                onClick={handleRollback}
                disabled={!tenantId || !canRollbackPolicies}
              >
                Rollback
              </Button>
            </div>
          </div>
          {actionError ? (
            <Alert variant="destructive">
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
        </CardContent>
      </Card>
      <Tabs value={viewMode} onValueChange={(value) => setViewMode(value as "builder" | "advanced")}>
        <TabsList>
          <TabsTrigger value="builder">Rule builder</TabsTrigger>
          <TabsTrigger value="advanced">Advanced (Rego)</TabsTrigger>
        </TabsList>
        <TabsContent value="builder" className="space-y-6">
          <CoveragePanel
            adminKey={adminKey ?? ""}
            tenantId={tenantId ?? ""}
            refreshToken={coverageRefresh}
            onConfigure={handleConfigureGap}
          />
          <RuleBuilder
            adminKey={adminKey ?? ""}
            tenantId={tenantId ?? ""}
            actionTypes={actionTypesList}
            tools={toolsList}
            canWrite={canWritePolicies}
            canPublish={canPublishPolicies}
            initialPolicy={structuredInitial}
            advancedActive={advancedActive}
            seedRule={seedRule}
            onSeedConsumed={() => setSeedRule(null)}
            onSaved={handleStructuredSaved}
          />
        </TabsContent>
        <TabsContent value="advanced">
      <Card>
        <CardHeader>
          <CardTitle>New policy version</CardTitle>
          <CardDescription>
            Start from the active policy, apply changes, validate, then publish.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="policy-version">Version</Label>
            <Input
              id="policy-version"
              value={policyVersion}
              onChange={(event) => setPolicyVersion(event.target.value)}
              placeholder="p_2026_01_21_001"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="policy-notes">Notes</Label>
            <Input
              id="policy-notes"
              value={policyNotes}
              onChange={(event) => setPolicyNotes(event.target.value)}
              placeholder="Reason for change"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="rego">Rego module (rego.v1)</Label>
            <Textarea
              id="rego"
              value={regoModule}
              onChange={(event) => setRegoModule(event.target.value)}
              className="min-h-[220px]"
              placeholder="package rbitr.policy\n\nimport rego.v1\n..."
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={handleCompileCheck}
              disabled={!tenantId || !canSimulatePolicies}
            >
              Run compile check
            </Button>
            <Button
              variant="outline"
              onClick={handleSimulate}
              disabled={!tenantId || !canSimulatePolicies}
            >
              Run simulation
            </Button>
          </div>
          <div className="space-y-2">
            <Label htmlFor="simulate-input">Simulation input (JSON)</Label>
            <Textarea
              id="simulate-input"
              value={simulateInput}
              onChange={(event) => setSimulateInput(event.target.value)}
              className="min-h-[120px]"
              placeholder='{"tenant_id":"t_demo"}'
            />
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            <div className="rounded-md border p-3 text-sm">
              <div className="text-xs text-muted-foreground">Compile check</div>
              <div className="mt-2">
                {compileStatus === "ok" ? (
                  <Badge className="bg-emerald-100 text-emerald-800">
                    Passed
                  </Badge>
                ) : compileStatus === "error" ? (
                  <Badge className="bg-rose-100 text-rose-800">Failed</Badge>
                ) : (
                  <Badge variant="outline">Not run</Badge>
                )}
              </div>
            </div>
            <div className="rounded-md border p-3 text-sm">
              <div className="text-xs text-muted-foreground">Simulation</div>
              <div className="mt-2">
                {simulateStatus === "ok" ? (
                  <Badge className="bg-emerald-100 text-emerald-800">
                    Passed
                  </Badge>
                ) : simulateStatus === "error" ? (
                  <Badge className="bg-rose-100 text-rose-800">Failed</Badge>
                ) : (
                  <Badge variant="outline">Not run</Badge>
                )}
              </div>
            </div>
            <div className="rounded-md border p-3 text-sm">
              <div className="text-xs text-muted-foreground">Diff preview</div>
              <div className="mt-1 font-medium">
                {diffPreview ? "Ready" : "Not available"}
              </div>
            </div>
          </div>
          <div className="rounded-md border p-3 text-xs">
            <div className="text-sm font-medium">Diff preview</div>
            <div className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap text-xs">
              {diffPreview
                ? diffPreview.split("\n").map((line, index) => {
                    let className = "text-muted-foreground";
                    if (line.startsWith("+")) {
                      className = "text-emerald-600 bg-emerald-50";
                    } else if (line.startsWith("-")) {
                      className = "text-rose-600 bg-rose-50";
                    }
                    return (
                      <div
                        key={`${line}-${index}`}
                        className={`px-2 py-0.5 ${className}`}
                      >
                        {line}
                      </div>
                    );
                  })
                : "No changes yet."}
            </div>
          </div>
          {simulateResult ? (
            <div className="rounded-md border p-3 text-sm">
              {simulationSummary ? (
                <div className="mb-3 rounded-md border bg-muted/30 p-3">
                  <div className="text-xs text-muted-foreground">
                    Effective decision
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">{simulationSummary.decision}</Badge>
                    <span className="text-xs text-muted-foreground">
                      Risk {simulationSummary.risk}
                    </span>
                  </div>
                  <div className="mt-3 text-xs text-muted-foreground">
                    Top matched rule
                  </div>
                  <div className="mt-1 font-mono text-xs">
                    {simulationSummary.rule.id} (priority{" "}
                    {simulationSummary.rule.priority}, effect{" "}
                    {simulationSummary.rule.effect})
                  </div>
                  {simulationSummary.rule.reasons.length > 0 ? (
                    <div className="mt-2 text-xs">
                      {simulationSummary.rule.reasons
                        .map((reason) => `${reason.code}: ${reason.message}`)
                        .join(" | ")}
                    </div>
                  ) : null}
                </div>
              ) : null}
              <div className="font-medium">Simulation result</div>
              <pre className="mt-2 whitespace-pre-wrap text-xs">
                {JSON.stringify(simulateResult.decision, null, 2)}
              </pre>
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button onClick={handleCreate} disabled={!tenantId || !canWritePolicies}>
              Save new version
            </Button>
            <Button
              onClick={handleCreateAndPublish}
              disabled={!tenantId || !publishReady || !canWritePolicies || !canPublishPolicies}
            >
              Create &amp; publish
            </Button>
            <Button variant="outline" onClick={refresh} disabled={!tenantId || !canReadPolicies}>
              Refresh list
            </Button>
          </div>
        </CardContent>
      </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
