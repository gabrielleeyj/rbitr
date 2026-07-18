import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { initializeSetup, type SetupInitializeResponse, type SetupStatus } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";

type SetupStep = "welcome" | "checks" | "configure" | "complete";

interface SetupPageProps {
  status: SetupStatus;
  onRefreshStatus: () => Promise<void> | void;
  onSetupCompleted: () => Promise<void> | void;
}

export function SetupPage({ status, onRefreshStatus, onSetupCompleted }: SetupPageProps) {
  const navigate = useNavigate();
  const { setAdminKey } = useAdminKey();
  const { setSelectedTenant } = useTenant();

  const [step, setStep] = useState<SetupStep>("welcome");
  const [tenantName, setTenantName] = useState("");
  const [tenantID, setTenantID] = useState("");
  const [adminKey, setAdminKeyInput] = useState("");
  const [tenantKey, setTenantKeyInput] = useState("");
  const [setupToken, setSetupToken] = useState("");
  const [idempotencyKey, setIdempotencyKey] = useState(generateIdempotencyKey());
  const [result, setResult] = useState<SetupInitializeResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const checks = useMemo(
    () => [
      { label: "Database connectivity", ok: status.database_reachable },
      { label: "Schema migrations", ok: status.schema_ready },
      { label: "Setup initialize endpoint", ok: status.initialize_allowed },
    ],
    [status.database_reachable, status.initialize_allowed, status.schema_ready]
  );

  const handleInitialize = async () => {
    if (!tenantName.trim()) {
      setError("Tenant name is required.");
      return;
    }
    if (status.initialize_token_required && !setupToken.trim()) {
      setError("Setup token is required.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const response = await initializeSetup({
        tenant_name: tenantName.trim(),
        tenant_id: tenantID.trim() || undefined,
        admin_key: adminKey.trim() || undefined,
        tenant_key: tenantKey.trim() || undefined,
      }, {
        setup_token: setupToken.trim() || undefined,
        idempotency_key: idempotencyKey,
      });
      setResult(response);
      setStep("complete");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Setup initialization failed.";
      setError(parseErrorMessage(message));
    } finally {
      setLoading(false);
    }
  };

  const handleEnterControlPlane = async () => {
    if (!result) return;
    setAdminKey(result.admin_key);
    setSelectedTenant({
      tenant_id: result.tenant_id,
      name: result.tenant_name,
      active_policy_version: result.policy_version,
      tool_count: 0,
    });
    await onSetupCompleted();
    navigate("/tenants", { replace: true });
  };

  return (
    <div className="min-h-screen bg-muted/30 flex items-center justify-center px-6 py-8">
      <Card className="w-full max-w-3xl">
        <CardHeader>
          <CardTitle>Welcome to rbitr</CardTitle>
          <CardDescription>Complete first-run setup to activate your control plane.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex flex-wrap gap-2">
            <Badge variant={step === "welcome" ? "default" : "secondary"}>1. Welcome</Badge>
            <Badge variant={step === "checks" ? "default" : "secondary"}>2. Environment</Badge>
            <Badge variant={step === "configure" ? "default" : "secondary"}>3. Configure</Badge>
            <Badge variant={step === "complete" ? "default" : "secondary"}>4. Complete</Badge>
          </div>

          {step === "welcome" ? (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                This wizard provisions your first tenant profile, creates admin and tenant API keys, seeds a default
                governance policy, and marks bootstrap complete.
              </p>
              <div className="flex justify-end">
                <Button onClick={() => setStep("checks")}>Next</Button>
              </div>
            </div>
          ) : null}

          {step === "checks" ? (
            <div className="space-y-4">
              {checks.map((check) => (
                <div key={check.label} className="flex items-center justify-between border rounded-md px-3 py-2">
                  <span className="text-sm">{check.label}</span>
                  <Badge variant={check.ok ? "success" : "danger"}>{check.ok ? "Ready" : "Not ready"}</Badge>
                </div>
              ))}

              <div className="flex items-center justify-between border rounded-md px-3 py-2">
                <span className="text-sm">Persisted setup state</span>
                <StatusBadge status={status.setup_state} />
              </div>

              {status.last_error ? (
                <Alert variant="destructive">
                  <AlertDescription>{status.last_error}</AlertDescription>
                </Alert>
              ) : null}

              {!status.schema_ready ? (
                <Alert variant="destructive">
                  <AlertDescription>
                    Schema is not ready. Run database migrations first, then refresh checks.
                  </AlertDescription>
                </Alert>
              ) : null}

              <div className="flex justify-between">
                <Button variant="outline" onClick={() => setStep("welcome")}>
                  Back
                </Button>
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => void onRefreshStatus()}>
                    Refresh checks
                  </Button>
                  <Button onClick={() => setStep("configure")} disabled={!status.schema_ready}>
                    Next
                  </Button>
                </div>
              </div>
            </div>
          ) : null}

          {step === "configure" ? (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="tenant-name">Tenant name</Label>
                <Input
                  id="tenant-name"
                  placeholder="Acme Corp"
                  value={tenantName}
                  onChange={(event) => {
                    setTenantName(event.target.value);
                    setIdempotencyKey(generateIdempotencyKey());
                  }}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="tenant-id">Tenant ID (optional)</Label>
                <Input
                  id="tenant-id"
                  placeholder="t_acme"
                  value={tenantID}
                  onChange={(event) => {
                    setTenantID(event.target.value);
                    setIdempotencyKey(generateIdempotencyKey());
                  }}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="admin-key">Admin key (optional)</Label>
                <Input
                  id="admin-key"
                  placeholder="Leave empty to auto-generate"
                  value={adminKey}
                  onChange={(event) => {
                    setAdminKeyInput(event.target.value);
                    setIdempotencyKey(generateIdempotencyKey());
                  }}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="tenant-key">Tenant API key (optional)</Label>
                <Input
                  id="tenant-key"
                  placeholder="Leave empty to auto-generate"
                  value={tenantKey}
                  onChange={(event) => {
                    setTenantKeyInput(event.target.value);
                    setIdempotencyKey(generateIdempotencyKey());
                  }}
                />
              </div>

              {status.initialize_token_required ? (
                <div className="space-y-2">
                  <Label htmlFor="setup-token">Setup token</Label>
                  <Input
                    id="setup-token"
                    type="password"
                    placeholder="Paste setup bootstrap token"
                    value={setupToken}
                    onChange={(event) => setSetupToken(event.target.value)}
                  />
                </div>
              ) : null}

              {error ? (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              ) : null}

              <div className="flex justify-between">
                <Button variant="outline" onClick={() => setStep("checks")} disabled={loading}>
                  Back
                </Button>
                <Button onClick={() => void handleInitialize()} disabled={loading}>
                  {loading ? "Running setup..." : "Run setup"}
                </Button>
              </div>
            </div>
          ) : null}

          {step === "complete" && result ? (
            <div className="space-y-4">
              <Alert>
                <AlertDescription>
                  Setup completed for tenant <span className="font-mono">{result.tenant_id}</span> with active policy{" "}
                  <span className="font-mono">{result.policy_version}</span>.
                </AlertDescription>
              </Alert>

              <div className="space-y-2">
                <Label>Admin key</Label>
                <pre className="text-xs rounded-md bg-muted p-3 overflow-x-auto">{result.admin_key}</pre>
              </div>
              <div className="space-y-2">
                <Label>Tenant key</Label>
                <pre className="text-xs rounded-md bg-muted p-3 overflow-x-auto">{result.tenant_key}</pre>
              </div>

              <div className="flex justify-end">
                <Button onClick={() => void handleEnterControlPlane()}>Enter control plane</Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function parseErrorMessage(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) {
    return "Setup initialization failed.";
  }
  try {
    const parsed = JSON.parse(trimmed) as { error?: string; fields?: Record<string, string> };
    if (parsed.fields && Object.keys(parsed.fields).length > 0) {
      const details = Object.entries(parsed.fields)
        .map(([field, value]) => `${field}: ${value}`)
        .join("; ");
      return details || parsed.error || "Setup initialization failed.";
    }
    if (parsed.error) {
      return parsed.error;
    }
  } catch {
    // Ignore parse failures and return the raw message.
  }
  return trimmed;
}

function generateIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `setup-${crypto.randomUUID()}`;
  }
  return `setup-${Date.now()}-${Math.floor(Math.random() * 1000000)}`;
}
