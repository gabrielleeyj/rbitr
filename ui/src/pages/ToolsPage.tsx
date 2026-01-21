import { useEffect, useMemo, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { listTools, updateTool, type ToolConfig } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";

export function ToolsPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [tools, setTools] = useState<ToolConfig[]>([]);
  const [edits, setEdits] = useState<Record<string, { baseUrl: string; authType: string; authValue: string }>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");

  const toolRows = useMemo(() => {
    return tools.map((tool) => ({
      ...tool,
      edit: edits[tool.tool_id] ?? { baseUrl: tool.base_url, authType: tool.auth_type, authValue: "" },
    }));
  }, [tools, edits]);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      if (!adminKey || !tenantId) {
        setLoading(false);
        return;
      }
      try {
        const data = await listTools({ adminKey }, tenantId);
        if (!mounted) return;
        setTools(data);
        setLoading(false);
      } catch (err) {
        if (!mounted) return;
        setError(err instanceof Error ? err.message : "Failed to load tools.");
        setLoading(false);
      }
    };
    load();
    return () => {
      mounted = false;
    };
  }, [adminKey, tenantId]);

  const handleEdit = (toolId: string, field: "baseUrl" | "authType" | "authValue", value: string) => {
    setEdits((prev) => ({
      ...prev,
      [toolId]: { ...(prev[toolId] ?? { baseUrl: "", authType: "", authValue: "" }), [field]: value },
    }));
  };

  const handleUpdate = async (tool: ToolConfig) => {
    if (!adminKey || !tenantId) return;
    setActionError("");
    const edit = edits[tool.tool_id] ?? { baseUrl: tool.base_url, authType: tool.auth_type, authValue: "" };
    try {
      await updateTool(
        { adminKey },
        tenantId,
        tool.tool_id,
        { base_url: edit.baseUrl, auth_type: edit.authType, auth_value: edit.authValue }
      );
      toast.success("Tool updated", { description: tool.tool_id });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to update tool.");
    }
  };

  const refresh = async () => {
    if (!adminKey || !tenantId) return;
    const data = await listTools({ adminKey }, tenantId);
    setTools(data);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tools</CardTitle>
        <CardDescription>Manage tool connectors and base URLs.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
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
        {!tenantId ? (
          <div className="text-sm text-muted-foreground">Select a tenant to view tools.</div>
        ) : loading ? (
          <div className="text-sm text-muted-foreground">Loading tools...</div>
        ) : toolRows.length === 0 ? (
          <div className="text-sm text-muted-foreground">No tools configured yet.</div>
        ) : (
          <div className="space-y-3">
            <div className="flex justify-end">
              <Button variant="outline" size="sm" onClick={refresh}>
                Refresh
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tool</TableHead>
                  <TableHead>Base URL</TableHead>
                  <TableHead>Auth type</TableHead>
                  <TableHead>Auth value</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {toolRows.map((tool) => (
                  <TableRow key={tool.tool_id}>
                    <TableCell className="font-medium">{tool.tool_id}</TableCell>
                    <TableCell>
                      <Input
                        value={tool.edit.baseUrl}
                        onChange={(event) => handleEdit(tool.tool_id, "baseUrl", event.target.value)}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={tool.edit.authType}
                        onChange={(event) => handleEdit(tool.tool_id, "authType", event.target.value)}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        type="password"
                        placeholder={tool.auth_set ? "••••••" : "Not set"}
                        value={tool.edit.authValue}
                        onChange={(event) => handleEdit(tool.tool_id, "authValue", event.target.value)}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <Button size="sm" onClick={() => handleUpdate(tool)}>
                        Update
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
        <div className="text-xs text-muted-foreground">
          Updating a tool replaces the base URL and auth settings. Leaving auth value blank will clear the stored secret.
        </div>
      </CardContent>
    </Card>
  );
}
