import { useEffect, useMemo, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  createMailingList,
  deleteMailingList,
  getNotificationConfig,
  getNotificationMetadata,
  listMailingLists,
  sendEmailTest,
  sendSlackTest,
  sendSlackBotTest,
  setEmailSecretRef,
  setSlackSecretRef,
  updateMailingList,
  updateNotificationConfig,
  listNotificationSuppressions,
} from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";
import {
  scopeNotificationsRead,
  scopeNotificationsTest,
  scopeNotificationsWrite,
} from "@/lib/scopes";

const emptyConfig = {
  slack_webhook_enabled: false,
  slack_webhook_default_channel: "",
  slack_bot_enabled: false,
  slack_bot_default_channel: "",
  email_enabled: false,
  email_provider: "",
  email_from: "",
  email_region: "",
  email_domain: "",
  email_default_mailing_list_id: "",
  notify_approval_expiring: true,
  notify_token_abuse: true,
  notify_policy_invalid: true,
};

export function NotificationsPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [config, setConfig] = useState(emptyConfig);
  const [status, setStatus] = useState({
    slack_webhook_configured: false,
    slack_bot_configured: false,
    email_configured: false,
  });
  const [mailingLists, setMailingLists] = useState<Array<{ mailing_list_id: string; name: string; description?: string }>>(
    []
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [slackSecretRef, setSlackSecretRefInput] = useState("");
  const [emailSecretRef, setEmailSecretRefInput] = useState("");
  const [listForm, setListForm] = useState({
    mailing_list_id: "",
    name: "",
    description: "",
    members: "",
  });
  const [suppressions, setSuppressions] = useState<
    Array<{
      dedup_key: string;
      channel: string;
      event_type: string;
      severity: string;
      resource_id?: string;
      last_seen_at: string;
      last_sent_at?: string;
      suppressed_until?: string;
      suppressed_count: number;
    }>
  >([]);
  const [suppressionFilters, setSuppressionFilters] = useState({
    event_type: "all",
    channel: "all",
    severity: "all",
  });
  const [notificationMeta, setNotificationMeta] = useState<{
    event_types: string[];
    severities: string[];
    channels: string[];
  } | null>(null);
  const canReadNotifications = hasScope(scopeNotificationsRead);
  const canWriteNotifications = hasScope(scopeNotificationsWrite);
  const canTestNotifications = hasScope(scopeNotificationsTest);

  const isEditingList = Boolean(listForm.mailing_list_id);

  const parsedMembers = useMemo(() => {
    return listForm.members
      .split(/[\n,]/)
      .map((item) => item.trim())
      .filter((item) => item.length > 0);
  }, [listForm.members]);

  const load = async () => {
    if (!adminKey || !tenantId || !canReadNotifications) return;
    const [configResult, lists, suppressionResult] = await Promise.allSettled([
      getNotificationConfig({ adminKey }, tenantId),
      listMailingLists({ adminKey }, tenantId),
      listNotificationSuppressions({ adminKey }, tenantId, { limit: 50 }),
    ]);
    if (configResult.status === "fulfilled") {
      const configData = configResult.value;
      setConfig({
        slack_webhook_enabled: configData.slack_webhook_enabled,
        slack_webhook_default_channel: configData.slack_webhook_default_channel ?? "",
        slack_bot_enabled: configData.slack_bot_enabled,
        slack_bot_default_channel: configData.slack_bot_default_channel ?? "",
        email_enabled: configData.email_enabled,
        email_provider: configData.email_provider ?? "",
        email_from: configData.email_from ?? "",
        email_region: configData.email_region ?? "",
        email_domain: configData.email_domain ?? "",
        email_default_mailing_list_id: configData.email_default_mailing_list_id ?? "",
        notify_approval_expiring: configData.notify_approval_expiring,
        notify_token_abuse: configData.notify_token_abuse,
        notify_policy_invalid: configData.notify_policy_invalid,
      });
      setStatus({
        slack_webhook_configured: configData.slack_webhook_configured,
        slack_bot_configured: configData.slack_bot_configured,
        email_configured: configData.email_configured,
      });
    } else {
      setConfig(emptyConfig);
      setStatus({
        slack_webhook_configured: false,
        slack_bot_configured: false,
        email_configured: false,
      });
    }
    if (lists.status === "fulfilled") {
      setMailingLists(lists.value);
    } else {
      setMailingLists([]);
    }
    if (suppressionResult.status === "fulfilled") {
      setSuppressions(suppressionResult.value);
    } else {
      setSuppressions([]);
    }
  };

  useEffect(() => {
    let mounted = true;
    const init = async () => {
      if (!adminKey || !tenantId || !canReadNotifications) {
        setConfig(emptyConfig);
        setStatus({
          slack_webhook_configured: false,
          slack_bot_configured: false,
          email_configured: false,
        });
        setMailingLists([]);
        setSuppressions([]);
        setLoading(false);
        return;
      }
      try {
        await load();
        if (!mounted) return;
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load notification settings.");
        setLoading(false);
      }
    };
    init();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId, canReadNotifications]);

  useEffect(() => {
    let mounted = true;
    const loadMetadata = async () => {
      if (!adminKey || !canReadNotifications) {
        if (mounted) {
          setNotificationMeta(null);
        }
        return;
      }
      try {
        const meta = await getNotificationMetadata({ adminKey });
        if (mounted) {
          setNotificationMeta(meta);
        }
      } catch {
        if (mounted) {
          setNotificationMeta(null);
        }
      }
    };
    loadMetadata();
    return () => {
      mounted = false;
    };
  }, [adminKey, canReadNotifications]);

  const handleConfigSave = async () => {
    if (!adminKey || !tenantId || !canWriteNotifications) return;
    setActionError("");
    try {
      await updateNotificationConfig({ adminKey }, tenantId, config);
      toast.success("Notification config updated");
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update notification config.");
    }
  };

  const handleSlackSecretSave = async () => {
    if (!adminKey || !tenantId || !slackSecretRef || !canWriteNotifications) return;
    setActionError("");
    try {
      await setSlackSecretRef({ adminKey }, tenantId, slackSecretRef);
      setSlackSecretRefInput("");
      toast.success("Slack secret ref saved");
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to set slack secret ref.");
    }
  };

  const handleEmailSecretSave = async () => {
    if (!adminKey || !tenantId || !emailSecretRef || !canWriteNotifications) return;
    setActionError("");
    try {
      await setEmailSecretRef({ adminKey }, tenantId, emailSecretRef);
      setEmailSecretRefInput("");
      toast.success("Email secret ref saved");
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to set email secret ref.");
    }
  };

  const handleSlackTest = async () => {
    if (!adminKey || !tenantId || !canTestNotifications) return;
    setActionError("");
    try {
      await sendSlackTest({ adminKey }, tenantId);
      toast.success("Slack test sent");
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to send slack test.");
    }
  };

  const handleSlackBotTest = async () => {
    if (!adminKey || !tenantId || !canTestNotifications) return;
    setActionError("");
    try {
      await sendSlackBotTest({ adminKey }, tenantId);
      toast.success("Slack bot test sent");
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to send slack bot test.");
    }
  };

  const handleEmailTest = async () => {
    if (!adminKey || !tenantId || !canTestNotifications) return;
    setActionError("");
    try {
      await sendEmailTest({ adminKey }, tenantId);
      toast.success("Email test sent");
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to send email test.");
    }
  };

  const handleListSubmit = async () => {
    if (!adminKey || !tenantId || !canWriteNotifications) return;
    setActionError("");
    if (!listForm.name) {
      setActionError("Mailing list name is required.");
      return;
    }
    try {
      if (isEditingList) {
        await updateMailingList({ adminKey }, tenantId, listForm.mailing_list_id, {
          name: listForm.name,
          description: listForm.description,
          members: parsedMembers,
        });
        toast.success("Mailing list updated");
      } else {
        await createMailingList({ adminKey }, tenantId, {
          name: listForm.name,
          description: listForm.description,
          members: parsedMembers,
        });
        toast.success("Mailing list created");
      }
      setListForm({ mailing_list_id: "", name: "", description: "", members: "" });
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to save mailing list.");
    }
  };

  const handleListEdit = (id: string, name: string, description?: string) => {
    setListForm({
      mailing_list_id: id,
      name,
      description: description ?? "",
      members: "",
    });
  };

  const handleListDelete = async (id: string) => {
    if (!adminKey || !tenantId || !canWriteNotifications) return;
    setActionError("");
    try {
      await deleteMailingList({ adminKey }, tenantId, id);
      toast.success("Mailing list deleted");
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to delete mailing list.");
    }
  };

  const resetListForm = () => {
    setListForm({ mailing_list_id: "", name: "", description: "", members: "" });
  };

  const refreshSuppressions = async () => {
    if (!adminKey || !tenantId || !canReadNotifications) return;
    try {
      const data = await listNotificationSuppressions({ adminKey }, tenantId, {
        limit: 50,
        event_type: suppressionFilters.event_type === "all" ? undefined : suppressionFilters.event_type,
        channel: suppressionFilters.channel === "all" ? undefined : suppressionFilters.channel,
        severity: suppressionFilters.severity === "all" ? undefined : suppressionFilters.severity,
      });
      setSuppressions(data);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to load suppressions.");
    }
  };

  return (
    <div className="space-y-6">
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {actionError ? (
        <Alert variant="destructive">
          <AlertDescription>{actionError}</AlertDescription>
        </Alert>
      ) : null}
      {!canReadNotifications ? (
        <Alert>
          <AlertDescription>Missing scope: {scopeNotificationsRead}</AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Notification routing</CardTitle>
          <CardDescription>Configure Slack and email delivery for governance alerts.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid gap-6 lg:grid-cols-2">
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-sm font-semibold">Slack webhook</div>
                  <div className="text-xs text-muted-foreground">
                    Configured: {status.slack_webhook_configured ? "Yes" : "No"}
                  </div>
                </div>
                <Switch
                  checked={config.slack_webhook_enabled}
                  onCheckedChange={(checked) =>
                    setConfig((prev) => ({ ...prev, slack_webhook_enabled: checked }))
                  }
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="slack-webhook-channel">Default channel (optional)</Label>
                <Input
                  id="slack-webhook-channel"
                  value={config.slack_webhook_default_channel}
                  onChange={(event) =>
                    setConfig((prev) => ({ ...prev, slack_webhook_default_channel: event.target.value }))
                  }
                  placeholder="#alerts"
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="slack-secret-ref">Slack secret ref</Label>
                <div className="flex flex-wrap gap-2">
                  <Input
                    id="slack-secret-ref"
                    value={slackSecretRef}
                    onChange={(event) => setSlackSecretRefInput(event.target.value)}
                    placeholder="env://RBTR_SLACK_WEBHOOK"
                    disabled={!canWriteNotifications}
                  />
                  <Button
                    variant="outline"
                    onClick={handleSlackSecretSave}
                    disabled={!slackSecretRef || !canWriteNotifications}
                  >
                    Save ref
                  </Button>
                </div>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={handleSlackTest}
                  disabled={!status.slack_webhook_configured || !canTestNotifications}
                >
                  Send Slack test
                </Button>
              </div>
            </div>

            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-sm font-semibold">Slack bot</div>
                  <div className="text-xs text-muted-foreground">Configured: {status.slack_bot_configured ? "Yes" : "No"}</div>
                </div>
                <Switch
                  checked={config.slack_bot_enabled}
                  onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, slack_bot_enabled: checked }))}
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="slack-bot-channel">Default channel ID</Label>
                <Input
                  id="slack-bot-channel"
                  value={config.slack_bot_default_channel}
                  onChange={(event) =>
                    setConfig((prev) => ({ ...prev, slack_bot_default_channel: event.target.value }))
                  }
                  placeholder="C01234567"
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={handleSlackBotTest}
                  disabled={!status.slack_bot_configured || !canTestNotifications}
                >
                  Send Slack bot test
                </Button>
              </div>
            </div>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-sm font-semibold">Email delivery</div>
                  <div className="text-xs text-muted-foreground">Configured: {status.email_configured ? "Yes" : "No"}</div>
                </div>
                <Switch
                  checked={config.email_enabled}
                  onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, email_enabled: checked }))}
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              <div className="space-y-2">
                <Label>Email provider</Label>
                <Select
                  value={config.email_provider || undefined}
                  onValueChange={(value) => setConfig((prev) => ({ ...prev, email_provider: value }))}
                  disabled={!canWriteNotifications}
                >
                  <SelectTrigger disabled={!canWriteNotifications}>
                    <SelectValue placeholder="Select provider" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ses">SES</SelectItem>
                    <SelectItem value="sendgrid">SendGrid</SelectItem>
                    <SelectItem value="mailgun">Mailgun</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="email-from">From address</Label>
                <Input
                  id="email-from"
                  value={config.email_from}
                  onChange={(event) => setConfig((prev) => ({ ...prev, email_from: event.target.value }))}
                  placeholder="alerts@example.com"
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              {config.email_provider === "ses" ? (
                <div className="space-y-2">
                  <Label htmlFor="email-region">Region (SES)</Label>
                  <Input
                    id="email-region"
                    value={config.email_region}
                    onChange={(event) => setConfig((prev) => ({ ...prev, email_region: event.target.value }))}
                    placeholder="us-east-1"
                    disabled={loading || !canWriteNotifications}
                  />
                </div>
              ) : null}
              {config.email_provider === "mailgun" ? (
                <div className="space-y-2">
                  <Label htmlFor="email-domain">Domain (Mailgun)</Label>
                  <Input
                    id="email-domain"
                    value={config.email_domain}
                    onChange={(event) => setConfig((prev) => ({ ...prev, email_domain: event.target.value }))}
                    placeholder="mg.example.com"
                    disabled={loading || !canWriteNotifications}
                  />
                </div>
              ) : null}
              <div className="space-y-2">
                <Label htmlFor="email-default-list">Default mailing list ID</Label>
                <Input
                  id="email-default-list"
                  value={config.email_default_mailing_list_id}
                  onChange={(event) =>
                    setConfig((prev) => ({ ...prev, email_default_mailing_list_id: event.target.value }))
                  }
                  placeholder="ml_security"
                  disabled={loading || !canWriteNotifications}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="email-secret-ref">Email secret ref</Label>
                <div className="flex flex-wrap gap-2">
                  <Input
                    id="email-secret-ref"
                    value={emailSecretRef}
                    onChange={(event) => setEmailSecretRefInput(event.target.value)}
                    placeholder="env://RBTR_EMAIL_PROVIDER_KEY"
                    disabled={!canWriteNotifications}
                  />
                  <Button
                    variant="outline"
                    onClick={handleEmailSecretSave}
                    disabled={!emailSecretRef || !canWriteNotifications}
                  >
                    Save ref
                  </Button>
                </div>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={handleEmailTest}
                  disabled={!status.email_configured || !canTestNotifications}
                >
                  Send Email test
                </Button>
              </div>
            </div>

            <div className="space-y-4">
              <div className="text-sm font-semibold">Event routing</div>
              <div className="flex items-center justify-between">
                <Label htmlFor="notify-approval-expiring" className="text-sm">
                  Approval expiring alerts
                </Label>
                <Switch
                  id="notify-approval-expiring"
                  checked={config.notify_approval_expiring}
                  onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, notify_approval_expiring: checked }))}
                  disabled={!canWriteNotifications}
                />
              </div>
              <div className="flex items-center justify-between">
                <Label htmlFor="notify-token-abuse" className="text-sm">
                  Token abuse alerts
                </Label>
                <Switch
                  id="notify-token-abuse"
                  checked={config.notify_token_abuse}
                  onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, notify_token_abuse: checked }))}
                  disabled={!canWriteNotifications}
                />
              </div>
              <div className="flex items-center justify-between">
                <Label htmlFor="notify-policy-invalid" className="text-sm">
                  Policy invalid alerts
                </Label>
                <Switch
                  id="notify-policy-invalid"
                  checked={config.notify_policy_invalid}
                  onCheckedChange={(checked) => setConfig((prev) => ({ ...prev, notify_policy_invalid: checked }))}
                  disabled={!canWriteNotifications}
                />
              </div>
              <div className="pt-2">
                <Button onClick={handleConfigSave} disabled={loading || !canWriteNotifications}>
                  Save notification config
                </Button>
              </div>
            </div>
          </div>

          {!tenantId ? (
            <div className="text-sm text-muted-foreground">Select a tenant to configure notifications.</div>
          ) : loading ? (
            <div className="text-sm text-muted-foreground">Loading notification configuration...</div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Mailing lists</CardTitle>
          <CardDescription>Create email recipient groups for future email notifications.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="mailing-name">List name</Label>
              <Input
                id="mailing-name"
                value={listForm.name}
                onChange={(event) => setListForm((prev) => ({ ...prev, name: event.target.value }))}
                disabled={!canWriteNotifications}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="mailing-description">Description</Label>
              <Input
                id="mailing-description"
                value={listForm.description}
                onChange={(event) => setListForm((prev) => ({ ...prev, description: event.target.value }))}
                disabled={!canWriteNotifications}
              />
            </div>
            <div className="md:col-span-2 space-y-2">
              <Label htmlFor="mailing-members">Members (comma or newline separated)</Label>
              <Textarea
                id="mailing-members"
                value={listForm.members}
                onChange={(event) => setListForm((prev) => ({ ...prev, members: event.target.value }))}
                placeholder="ops@example.com, security@example.com"
                rows={3}
                disabled={!canWriteNotifications}
              />
              {isEditingList ? (
                <div className="text-xs text-muted-foreground">
                  Updating a list replaces all members. Enter the full member list.
                </div>
              ) : null}
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={handleListSubmit} disabled={!tenantId || !canWriteNotifications}>
                {isEditingList ? "Update list" : "Create list"}
              </Button>
              {isEditingList ? (
                <Button variant="outline" onClick={resetListForm} disabled={!canWriteNotifications}>
                  Cancel edit
                </Button>
              ) : null}
            </div>
          </div>

          {!tenantId ? (
            <div className="text-sm text-muted-foreground">Select a tenant to view mailing lists.</div>
          ) : mailingLists.length === 0 ? (
            <div className="text-sm text-muted-foreground">No mailing lists configured.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead>List ID</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {mailingLists.map((list) => (
                  <TableRow key={list.mailing_list_id}>
                    <TableCell className="font-medium">{list.name}</TableCell>
                    <TableCell>{list.description || "—"}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{list.mailing_list_id}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => handleListEdit(list.mailing_list_id, list.name, list.description)}
                          disabled={!canWriteNotifications}
                        >
                          Edit
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => handleListDelete(list.mailing_list_id)}
                          disabled={!canWriteNotifications}
                        >
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Notification suppressions</CardTitle>
          <CardDescription>Dedup and suppression state for recent notifications.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 md:grid-cols-4">
            <div className="space-y-2">
              <Label htmlFor="suppression-event">Event type</Label>
              <Select
                value={suppressionFilters.event_type}
                onValueChange={(value) => setSuppressionFilters((prev) => ({ ...prev, event_type: value }))}
                disabled={!canReadNotifications}
              >
                <SelectTrigger id="suppression-event" disabled={!canReadNotifications}>
                  <SelectValue placeholder="All events" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  {(notificationMeta?.event_types ?? []).map((eventType) => (
                    <SelectItem key={eventType} value={eventType}>
                      {eventType}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="suppression-channel">Channel</Label>
              <Select
                value={suppressionFilters.channel}
                onValueChange={(value) => setSuppressionFilters((prev) => ({ ...prev, channel: value }))}
                disabled={!canReadNotifications}
              >
                <SelectTrigger id="suppression-channel" disabled={!canReadNotifications}>
                  <SelectValue placeholder="All channels" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="slack_webhook">slack_webhook</SelectItem>
                  <SelectItem value="slack_bot">slack_bot</SelectItem>
                  <SelectItem value="email">email</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="suppression-severity">Severity</Label>
              <Select
                value={suppressionFilters.severity}
                onValueChange={(value) => setSuppressionFilters((prev) => ({ ...prev, severity: value }))}
                disabled={!canReadNotifications}
              >
                <SelectTrigger id="suppression-severity" disabled={!canReadNotifications}>
                  <SelectValue placeholder="All severities" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="INFO">INFO</SelectItem>
                  <SelectItem value="WARN">WARN</SelectItem>
                  <SelectItem value="CRITICAL">CRITICAL</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end gap-2">
              <Button variant="outline" onClick={refreshSuppressions} disabled={!tenantId || !canReadNotifications}>
                Refresh
              </Button>
            </div>
          </div>

          {!tenantId ? (
            <div className="text-sm text-muted-foreground">Select a tenant to view suppressions.</div>
          ) : suppressions.length === 0 ? (
            <div className="text-sm text-muted-foreground">No suppressions recorded.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Event</TableHead>
                  <TableHead>Channel</TableHead>
                  <TableHead>Severity</TableHead>
                  <TableHead>Last seen</TableHead>
                  <TableHead>Suppressed count</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {suppressions.map((item) => (
                  <TableRow key={item.dedup_key}>
                    <TableCell className="font-medium">{item.event_type}</TableCell>
                    <TableCell>{item.channel}</TableCell>
                    <TableCell>{item.severity}</TableCell>
                    <TableCell>{item.last_seen_at ? new Date(item.last_seen_at).toLocaleString() : "—"}</TableCell>
                    <TableCell>{item.suppressed_count}</TableCell>
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
