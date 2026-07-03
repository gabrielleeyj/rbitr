import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  saveStructuredPolicy,
  type PolicyEffect,
  type StructuredPolicy,
  type StructuredRule,
} from "@/lib/api";

const EFFECTS: PolicyEffect[] = ["ALLOW", "DENY", "REQUIRE_APPROVAL"];
const RISKS = ["LOW", "MEDIUM", "HIGH", "CRITICAL"];
const MCP_PREFIX = "MCP.*";

const EFFECT_VARIANT: Record<PolicyEffect, "default" | "secondary" | "destructive"> = {
  ALLOW: "secondary",
  REQUIRE_APPROVAL: "default",
  DENY: "destructive",
};

export const emptyStructuredPolicy: StructuredPolicy = {
  schema_version: "1",
  default_effect: "DENY",
  rules: [],
};

interface RuleBuilderProps {
  adminKey: string;
  tenantId: string;
  actionTypes: string[];
  tools: string[];
  canWrite: boolean;
  canPublish: boolean;
  initialPolicy: StructuredPolicy | null;
  advancedActive: boolean;
  seedRule: StructuredRule | null;
  onSeedConsumed: () => void;
  onSaved: () => void;
}

function generateVersion(): string {
  const now = new Date();
  const pad = (value: number) => value.toString().padStart(2, "0");
  return `p_${now.getUTCFullYear()}_${pad(now.getUTCMonth() + 1)}_${pad(now.getUTCDate())}_${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}`;
}

function nextRuleId(rules: StructuredRule[]): string {
  const base = "rule";
  let index = rules.length + 1;
  const existing = new Set(rules.map((r) => r.id));
  while (existing.has(`${base}_${index}`)) {
    index += 1;
  }
  return `${base}_${index}`;
}

/** ChipSelect renders a toggle-chip multi-select for a set of options. */
interface ChipSelectProps {
  label: string;
  options: string[];
  selected: string[];
  onToggle: (value: string) => void;
  emptyHint: string;
}

function ChipSelect({ label, options, selected, onToggle, emptyHint }: ChipSelectProps) {
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <div className="flex flex-wrap gap-1">
        {options.length === 0 ? (
          <span className="text-xs text-muted-foreground">{emptyHint}</span>
        ) : (
          options.map((option) => {
            const isSelected = selected.includes(option);
            return (
              <button
                key={option}
                type="button"
                onClick={() => onToggle(option)}
                className={`rounded-full border px-2 py-0.5 text-xs transition-colors ${
                  isSelected
                    ? "border-primary bg-primary text-primary-foreground"
                    : "border-border bg-background text-muted-foreground hover:bg-muted"
                }`}
              >
                {option}
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}

export function RuleBuilder({
  adminKey,
  tenantId,
  actionTypes,
  tools,
  canWrite,
  canPublish,
  initialPolicy,
  advancedActive,
  seedRule,
  onSeedConsumed,
  onSaved,
}: RuleBuilderProps) {
  const [policy, setPolicy] = useState<StructuredPolicy>(
    initialPolicy ?? emptyStructuredPolicy,
  );
  const [notes, setNotes] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setPolicy(initialPolicy ?? emptyStructuredPolicy);
  }, [initialPolicy]);

  // Append a rule seeded by the coverage panel's "Set permission" action.
  useEffect(() => {
    if (!seedRule) {
      return;
    }
    setPolicy((prev) => ({
      ...prev,
      rules: [...prev.rules, { ...seedRule, id: nextRuleId(prev.rules) }],
    }));
    onSeedConsumed();
  }, [seedRule, onSeedConsumed]);

  const actionOptions = useMemo(() => [...actionTypes, MCP_PREFIX], [actionTypes]);

  const updateRule = (index: number, patch: Partial<StructuredRule>) => {
    setPolicy((prev) => ({
      ...prev,
      rules: prev.rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)),
    }));
  };

  const toggleMatchValue = (
    index: number,
    field: "tool_ids" | "action_types" | "action_risks",
    value: string,
  ) => {
    setPolicy((prev) => ({
      ...prev,
      rules: prev.rules.map((rule, i) => {
        if (i !== index) {
          return rule;
        }
        const current = rule.match[field] ?? [];
        const next = current.includes(value)
          ? current.filter((v) => v !== value)
          : [...current, value];
        return { ...rule, match: { ...rule.match, [field]: next } };
      }),
    }));
  };

  const addRule = () => {
    setPolicy((prev) => ({
      ...prev,
      rules: [
        ...prev.rules,
        {
          id: nextRuleId(prev.rules),
          priority: Math.max(0, 100 - prev.rules.length * 10),
          effect: "ALLOW",
          match: {},
        },
      ],
    }));
  };

  const removeRule = (index: number) => {
    setPolicy((prev) => ({
      ...prev,
      rules: prev.rules.filter((_, i) => i !== index),
    }));
  };

  const moveRule = (index: number, direction: -1 | 1) => {
    setPolicy((prev) => {
      const target = index + direction;
      if (target < 0 || target >= prev.rules.length) {
        return prev;
      }
      const rules = [...prev.rules];
      [rules[index], rules[target]] = [rules[target], rules[index]];
      return { ...prev, rules };
    });
  };

  const save = async (publish: boolean) => {
    if (!canWrite) {
      return;
    }
    setSaving(true);
    try {
      await saveStructuredPolicy({ adminKey }, tenantId, {
        policy_version: generateVersion(),
        notes: notes.trim() || undefined,
        structured: policy,
        publish,
      });
      toast.success(publish ? "Policy saved and published" : "Policy version saved");
      setNotes("");
      onSaved();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save policy");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Permission rules</CardTitle>
        <CardDescription>
          Rules are evaluated from highest priority to lowest; the first match wins. Anything
          that matches no rule falls back to the default below.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {advancedActive ? (
          <Alert>
            <AlertDescription>
              The active policy version was authored as raw Rego. Saving here creates a new
              structured version that will replace it when published.
            </AlertDescription>
          </Alert>
        ) : null}

        <div className="flex items-center gap-2">
          <Label className="text-sm">Default when no rule matches</Label>
          <Select
            value={policy.default_effect}
            onValueChange={(value) =>
              setPolicy((prev) => ({ ...prev, default_effect: value as PolicyEffect }))
            }
          >
            <SelectTrigger className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EFFECTS.map((effect) => (
                <SelectItem key={effect} value={effect}>
                  {effect}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-3">
          {policy.rules.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No rules yet. Add a rule, or configure an endpoint from the coverage list.
            </p>
          ) : (
            policy.rules.map((rule, index) => (
              <div key={rule.id} className="space-y-3 rounded-md border p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={EFFECT_VARIANT[rule.effect]}>{rule.effect}</Badge>
                  <Input
                    aria-label="Rule id"
                    className="h-8 w-40"
                    value={rule.id}
                    onChange={(e) => updateRule(index, { id: e.target.value })}
                  />
                  <Select
                    value={rule.effect}
                    onValueChange={(value) => updateRule(index, { effect: value as PolicyEffect })}
                  >
                    <SelectTrigger className="h-8 w-44">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {EFFECTS.map((effect) => (
                        <SelectItem key={effect} value={effect}>
                          {effect}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <div className="flex items-center gap-1">
                    <Label className="text-xs text-muted-foreground">Priority</Label>
                    <Input
                      aria-label="Rule priority"
                      type="number"
                      className="h-8 w-20"
                      value={rule.priority}
                      onChange={(e) =>
                        updateRule(index, { priority: Number(e.target.value) || 0 })
                      }
                    />
                  </div>
                  <div className="ml-auto flex gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => moveRule(index, -1)}
                      disabled={index === 0}
                    >
                      ↑
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => moveRule(index, 1)}
                      disabled={index === policy.rules.length - 1}
                    >
                      ↓
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => removeRule(index)}
                    >
                      Remove
                    </Button>
                  </div>
                </div>

                <div className="grid gap-3 md:grid-cols-3">
                  <ChipSelect
                    label="Tools (any)"
                    options={tools}
                    selected={rule.match.tool_ids ?? []}
                    onToggle={(value) => toggleMatchValue(index, "tool_ids", value)}
                    emptyHint="No tools registered"
                  />
                  <ChipSelect
                    label="Actions (any)"
                    options={actionOptions}
                    selected={rule.match.action_types ?? []}
                    onToggle={(value) => toggleMatchValue(index, "action_types", value)}
                    emptyHint="No action types"
                  />
                  <ChipSelect
                    label="Risk (any)"
                    options={RISKS}
                    selected={rule.match.action_risks ?? []}
                    onToggle={(value) => toggleMatchValue(index, "action_risks", value)}
                    emptyHint=""
                  />
                </div>

                {rule.effect === "REQUIRE_APPROVAL" ? (
                  <Input
                    aria-label="Approval reason"
                    className="h-8"
                    placeholder="Approval reason (optional)"
                    value={rule.approval?.reason ?? ""}
                    onChange={(e) =>
                      updateRule(index, {
                        approval: { ...rule.approval, reason: e.target.value },
                      })
                    }
                  />
                ) : null}
              </div>
            ))
          )}
        </div>

        <Button type="button" variant="outline" size="sm" onClick={addRule} disabled={!canWrite}>
          Add rule
        </Button>

        <div className="space-y-1">
          <Label htmlFor="structured-notes">Notes</Label>
          <Input
            id="structured-notes"
            placeholder="What changed and why"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
          />
        </div>

        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={() => save(false)} disabled={!canWrite || saving}>
            Save version
          </Button>
          <Button type="button" onClick={() => save(true)} disabled={!canWrite || !canPublish || saving}>
            Save &amp; publish
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
