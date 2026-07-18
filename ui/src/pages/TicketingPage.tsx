import { useEffect, useState } from "react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/status-badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getTicketingConfig,
  updateTicketingConfig,
  setTicketingSecretRef,
  setTicketingWebhookSecretRef,
  sendTicketingTest,
  listTicketLinks,
} from "@/lib/api";
import type { TicketingConfig, TicketLink } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { useEntitlements } from "@/lib/entitlements";
import { toast } from "sonner";
import {
  scopeTicketingRead,
  scopeTicketingWrite,
  scopeTicketingTest,
} from "@/lib/scopes";

const emptyConfig = {
  provider: "",
  enabled: false,
  base_url: "",
  project_key: "",
  issue_type: "",
  auto_create: false,
};

export function TicketingPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const { hasFeature } = useEntitlements();
  const tenantId = selectedTenant?.tenant_id;

  const canRead = hasScope(scopeTicketingRead);
  const canWrite = hasScope(scopeTicketingWrite);
  const canTest = hasScope(scopeTicketingTest);
  const hasIntegrations = hasFeature("integrations");

  const [config, setConfig] = useState(emptyConfig);
  const [secretConfigured, setSecretConfigured] = useState(false);
  const [webhookSecretConfigured, setWebhookSecretConfigured] = useState(false);
  const [links, setLinks] = useState<TicketLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [secretRef, setSecretRefInput] = useState("");
  const [webhookSecretRef, setWebhookSecretRefInput] = useState("");
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    if (!tenantId || !adminKey || !canRead) {
      setLoading(false);
      return;
    }
    let mounted = true;
    const load = async () => {
      setLoading(true);
      setError("");
      try {
        const [cfgResult, linksResult] = await Promise.allSettled([
          getTicketingConfig({ adminKey }, tenantId),
          listTicketLinks({ adminKey }, tenantId, { limit: 50 }),
        ]);

        if (!mounted) return;

        if (cfgResult.status === "fulfilled") {
          const c = cfgResult.value;
          setConfig({
            provider: c.provider || "",
            enabled: c.enabled,
            base_url: c.base_url || "",
            project_key: c.project_key || "",
            issue_type: c.issue_type || "",
            auto_create: c.auto_create,
          });
          setSecretConfigured(c.secret_configured);
          setWebhookSecretConfigured(c.webhook_secret_configured);
        } else {
          setError("Failed to load ticketing configuration.");
        }

        if (linksResult.status === "fulfilled") {
          setLinks(linksResult.value);
        }
      } catch {
        if (mounted) setError("Failed to load ticketing data.");
      } finally {
        if (mounted) setLoading(false);
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [tenantId, adminKey, canRead]);

  const handleSaveConfig = async () => {
    if (!tenantId || !adminKey) return;
    setSaving(true);
    setActionError("");
    try {
      await updateTicketingConfig({ adminKey }, tenantId, config);
      toast.success("Ticketing configuration saved.");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to save config.";
      setActionError(msg);
      toast.error("Failed to save ticketing configuration.");
    } finally {
      setSaving(false);
    }
  };

  const handleSetSecretRef = async () => {
    if (!tenantId || !adminKey || !secretRef.trim()) return;
    setActionError("");
    try {
      await setTicketingSecretRef({ adminKey }, tenantId, secretRef.trim());
      setSecretConfigured(true);
      setSecretRefInput("");
      toast.success("API token secret reference saved.");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to set secret ref.";
      setActionError(msg);
      toast.error("Failed to set API token secret reference.");
    }
  };

  const handleSetWebhookSecretRef = async () => {
    if (!tenantId || !adminKey || !webhookSecretRef.trim()) return;
    setActionError("");
    try {
      await setTicketingWebhookSecretRef({ adminKey }, tenantId, webhookSecretRef.trim());
      setWebhookSecretConfigured(true);
      setWebhookSecretRefInput("");
      toast.success("Webhook secret reference saved.");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to set webhook secret.";
      setActionError(msg);
      toast.error("Failed to set webhook secret reference.");
    }
  };

  const handleTestSend = async () => {
    if (!tenantId || !adminKey) return;
    setTesting(true);
    setActionError("");
    try {
      await sendTicketingTest({ adminKey }, tenantId);
      toast.success("Test ticket creation initiated.");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create test ticket.";
      setActionError(msg);
      toast.error("Failed to create test ticket.");
    } finally {
      setTesting(false);
    }
  };

  if (!hasIntegrations) {
    return (
      <div className="p-6 space-y-4">
        <div>
          <h2 className="font-display text-2xl font-bold tracking-tight">Ticketing & ITSM</h2>
          <p className="text-sm text-muted-foreground">
            Configure bidirectional ticketing integration with Jira, ServiceNow, or Linear.
          </p>
        </div>
        <Alert>
          <AlertDescription>
            Ticketing integrations are not available on the free tier. Upload a license key to unlock this feature.
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  if (!tenantId) {
    return (
      <div className="p-6">
        <Alert>
          <AlertDescription>Select a tenant to configure ticketing.</AlertDescription>
        </Alert>
      </div>
    );
  }

  if (!canRead) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertDescription>You do not have permission to view ticketing configuration.</AlertDescription>
        </Alert>
      </div>
    );
  }

  if (loading) {
    return <div className="p-6 text-sm text-muted-foreground">Loading ticketing configuration...</div>;
  }

  if (error) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-4xl">
      <div>
        <h2 className="font-display text-2xl font-bold tracking-tight">Ticketing & ITSM</h2>
        <p className="text-sm text-muted-foreground">
          Configure bidirectional ticketing integration with Jira, ServiceNow, or Linear.
        </p>
      </div>

      {actionError ? (
        <Alert variant="destructive">
          <AlertDescription>{actionError}</AlertDescription>
        </Alert>
      ) : null}

      {/* Provider Configuration */}
      <Card>
        <CardHeader>
          <CardTitle>Provider Configuration</CardTitle>
          <CardDescription>
            Select a ticketing provider and configure connection settings.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-4">
            <Label htmlFor="ticketing-enabled" className="min-w-24">Enabled</Label>
            <Switch
              id="ticketing-enabled"
              checked={config.enabled}
              disabled={!canWrite || saving}
              onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, enabled: checked }))}
            />
          </div>

          <div className="flex items-center gap-4">
            <Label htmlFor="ticketing-provider" className="min-w-24">Provider</Label>
            <Select
              value={config.provider}
              disabled={!canWrite || saving}
              onValueChange={(value) => setConfig((prev) => ({ ...prev, provider: value }))}
            >
              <SelectTrigger id="ticketing-provider" className="w-48">
                <SelectValue placeholder="Select provider" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="jira">Jira</SelectItem>
                <SelectItem value="servicenow">ServiceNow</SelectItem>
                <SelectItem value="linear">Linear</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center gap-4">
            <Label htmlFor="ticketing-base-url" className="min-w-24">Base URL</Label>
            <Input
              id="ticketing-base-url"
              placeholder={config.provider === "linear" ? "Not required for Linear" : "https://your-instance.atlassian.net"}
              value={config.base_url}
              disabled={!canWrite || saving}
              onChange={(e) => setConfig((prev) => ({ ...prev, base_url: e.target.value }))}
              className="max-w-md"
            />
          </div>

          <div className="flex items-center gap-4">
            <Label htmlFor="ticketing-project-key" className="min-w-24">Project Key</Label>
            <Input
              id="ticketing-project-key"
              placeholder={config.provider === "servicenow" ? "Assignment group" : "e.g. RBITR"}
              value={config.project_key}
              disabled={!canWrite || saving}
              onChange={(e) => setConfig((prev) => ({ ...prev, project_key: e.target.value }))}
              className="max-w-md"
            />
          </div>

          <div className="flex items-center gap-4">
            <Label htmlFor="ticketing-issue-type" className="min-w-24">Issue Type</Label>
            <Input
              id="ticketing-issue-type"
              placeholder={config.provider === "servicenow" ? "e.g. incident" : "e.g. Task"}
              value={config.issue_type}
              disabled={!canWrite || saving}
              onChange={(e) => setConfig((prev) => ({ ...prev, issue_type: e.target.value }))}
              className="max-w-md"
            />
          </div>

          <div className="flex items-center gap-4">
            <Label htmlFor="ticketing-auto-create" className="min-w-24">Auto-create</Label>
            <Switch
              id="ticketing-auto-create"
              checked={config.auto_create}
              disabled={!canWrite || saving}
              onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, auto_create: checked }))}
            />
            <span className="text-xs text-muted-foreground">
              Automatically create tickets on REQUIRE_APPROVAL decisions
            </span>
          </div>

          {canWrite ? (
            <div className="pt-2">
              <Button onClick={() => void handleSaveConfig()} disabled={saving}>
                {saving ? "Saving..." : "Save Configuration"}
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {/* Secrets */}
      <Card>
        <CardHeader>
          <CardTitle>Credentials</CardTitle>
          <CardDescription>
            Configure API token and webhook signing secret references.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Label>API Token</Label>
              <Badge variant={secretConfigured ? "default" : "secondary"}>
                {secretConfigured ? "Configured" : "Not configured"}
              </Badge>
            </div>
            {canWrite ? (
              <div className="flex items-center gap-2">
                <Input
                  placeholder="e.g. env://JIRA_API_TOKEN"
                  value={secretRef}
                  onChange={(e) => setSecretRefInput(e.target.value)}
                  className="max-w-md"
                />
                <Button
                  variant="outline"
                  onClick={() => void handleSetSecretRef()}
                  disabled={!secretRef.trim()}
                >
                  Set
                </Button>
              </div>
            ) : null}
            <p className="text-xs text-muted-foreground">
              For Jira Basic Auth, use format: email:token (e.g. env://JIRA_EMAIL_TOKEN)
            </p>
          </div>

          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Label>Webhook Signing Secret</Label>
              <Badge variant={webhookSecretConfigured ? "default" : "secondary"}>
                {webhookSecretConfigured ? "Configured" : "Not configured"}
              </Badge>
            </div>
            {canWrite ? (
              <div className="flex items-center gap-2">
                <Input
                  placeholder="e.g. env://TICKETING_WEBHOOK_SECRET"
                  value={webhookSecretRef}
                  onChange={(e) => setWebhookSecretRefInput(e.target.value)}
                  className="max-w-md"
                />
                <Button
                  variant="outline"
                  onClick={() => void handleSetWebhookSecretRef()}
                  disabled={!webhookSecretRef.trim()}
                >
                  Set
                </Button>
              </div>
            ) : null}
            <p className="text-xs text-muted-foreground">
              Used to verify inbound webhook signatures (HMAC-SHA256).
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Test */}
      {canTest ? (
        <Card>
          <CardHeader>
            <CardTitle>Test Integration</CardTitle>
            <CardDescription>
              Create a test ticket to verify the configuration.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              onClick={() => void handleTestSend()}
              disabled={testing || !config.enabled || !config.provider || !secretConfigured}
            >
              {testing ? "Creating..." : "Create Test Ticket"}
            </Button>
            {!config.enabled || !config.provider || !secretConfigured ? (
              <p className="text-xs text-muted-foreground mt-2">
                Enable ticketing, select a provider, and configure an API token before testing.
              </p>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      {/* Ticket Links */}
      <Card>
        <CardHeader>
          <CardTitle>Ticket Links</CardTitle>
          <CardDescription>
            Approval requests linked to external tickets.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {links.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No ticket links yet. Links appear when approval requests create or
              attach tickets in your ticketing system.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Approval ID</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>External Key</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {links.map((link) => (
                  <TableRow key={link.ticket_link_id}>
                    <TableCell className="font-mono text-xs">{link.approval_request_id}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{link.provider}</Badge>
                    </TableCell>
                    <TableCell>
                      {link.external_url ? (
                        <a
                          href={link.external_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-primary hover:underline"
                        >
                          {link.external_key}
                        </a>
                      ) : (
                        link.external_key
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={link.status} />
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(link.created_at).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

