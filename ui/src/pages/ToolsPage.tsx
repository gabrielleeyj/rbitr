import { useEffect, useState } from "react";
import { ListSkeleton } from "@/components/list-skeleton";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { listTools, updateTool, updateToolMetadata, createTool, archiveTool, restoreTool, type ToolConfig } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";
import { scopeToolsRead, scopeToolsWrite } from "@/lib/scopes";

export function ToolsPage() {
  const { adminKey, hasScope } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [tools, setTools] = useState<ToolConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // HTTP tool editing
  const [httpEdits, setHttpEdits] = useState<Record<string, { baseUrl: string; authType: string; authValue: string }>>({});

  // HTTP tool create dialog
  const [httpCreateDialogOpen, setHttpCreateDialogOpen] = useState(false);
  const [httpCreateForm, setHttpCreateForm] = useState({
    tool_id: "",
    base_url: "",
    auth_type: "none",
    auth_value: "",
    description: "",
  });

  // MCP dialog state
  const [mcpDialogOpen, setMcpDialogOpen] = useState(false);
  const [mcpCreateMode, setMcpCreateMode] = useState(false);
  const [selectedTool, setSelectedTool] = useState<ToolConfig | null>(null);
  const [mcpForm, setMcpForm] = useState({
    tool_id: "",
    description: "",
    upstream_url: "",
    input_schema_json: "",
  });
  const [schemaError, setSchemaError] = useState("");
  const [showArchived, setShowArchived] = useState(false);

  // Archive confirmation dialog
  const [archiveDialogOpen, setArchiveDialogOpen] = useState(false);
  const [toolToArchive, setToolToArchive] = useState<string | null>(null);

  const canReadTools = hasScope(scopeToolsRead);
  const canWriteTools = hasScope(scopeToolsWrite);

  useEffect(() => {
    loadTools();
  }, [adminKey, tenantId, canReadTools]);

  const loadTools = async () => {
    if (!adminKey || !tenantId || !canReadTools) {
      setTools([]);
      setLoading(false);
      return;
    }
    try {
      const data = await listTools({ adminKey }, tenantId);
      setTools(data);
      setLoading(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load tools.");
      setLoading(false);
    }
  };

  const activeTools = tools.filter(t => !t.archived_at);
  const archivedTools = tools.filter(t => t.archived_at);
  const httpTools = activeTools.filter(t => t.http);
  // Show only tools with MCP configuration that have an upstream URL configured
  const mcpTools = activeTools.filter(t => t.mcp && t.mcp.upstream_url && t.mcp.upstream_url.trim() !== "");

  const handleHttpEdit = (toolId: string, field: "baseUrl" | "authType" | "authValue", value: string) => {
    setHttpEdits((prev) => ({
      ...prev,
      [toolId]: { ...(prev[toolId] ?? { baseUrl: "", authType: "", authValue: "" }), [field]: value },
    }));
  };

  const handleHttpUpdate = async (tool: ToolConfig) => {
    if (!adminKey || !tenantId || !canWriteTools) return;
    const edit = httpEdits[tool.tool_id] ?? {
      baseUrl: tool.http?.base_url || "",
      authType: tool.http?.auth_type || "",
      authValue: ""
    };
    try {
      await updateTool(
        { adminKey },
        tenantId,
        tool.tool_id,
        { base_url: edit.baseUrl, auth_type: edit.authType, auth_value: edit.authValue }
      );
      toast.success("HTTP tool updated", { description: tool.tool_id });
      await loadTools();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update tool");
    }
  };

  const openMcpCreateDialog = () => {
    if (!canWriteTools) return;
    setMcpCreateMode(true);
    setSelectedTool(null);
    setMcpForm({
      tool_id: "",
      description: "",
      upstream_url: "",
      input_schema_json: "",
    });
    setSchemaError("");
    setMcpDialogOpen(true);
  };

  const openMcpEditDialog = (tool: ToolConfig) => {
    if (!canWriteTools) return;
    setMcpCreateMode(false);
    setSelectedTool(tool);
    setMcpForm({
      tool_id: tool.tool_id,
      description: tool.mcp?.description || "",
      upstream_url: tool.mcp?.upstream_url || "",
      input_schema_json: tool.mcp?.input_schema_json ? JSON.stringify(tool.mcp.input_schema_json, null, 2) : "",
    });
    setSchemaError("");
    setMcpDialogOpen(true);
  };

  const handleHttpCreate = async () => {
    if (!adminKey || !tenantId || !canWriteTools) return;

    if (!httpCreateForm.tool_id.trim()) {
      toast.error("Tool ID is required");
      return;
    }
    if (!httpCreateForm.base_url.trim()) {
      toast.error("Base URL is required");
      return;
    }

    try {
      await createTool({ adminKey }, tenantId, {
        tool_id: httpCreateForm.tool_id,
        base_url: httpCreateForm.base_url,
        auth_type: httpCreateForm.auth_type,
        auth_value: httpCreateForm.auth_value,
        description: httpCreateForm.description,
        transport: "http",
      });
      toast.success("HTTP tool created", { description: httpCreateForm.tool_id });
      setHttpCreateDialogOpen(false);
      setHttpCreateForm({
        tool_id: "",
        base_url: "",
        auth_type: "none",
        auth_value: "",
        description: "",
      });
      await loadTools();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create tool");
    }
  };

  const openArchiveDialog = (toolId: string) => {
    setToolToArchive(toolId);
    setArchiveDialogOpen(true);
  };

  const handleArchive = async () => {
    if (!adminKey || !tenantId || !canWriteTools || !toolToArchive) return;

    try {
      await archiveTool({ adminKey }, tenantId, toolToArchive);
      toast.success("Tool archived", { description: toolToArchive });
      setArchiveDialogOpen(false);
      setToolToArchive(null);
      await loadTools();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to archive tool");
    }
  };

  const handleRestore = async (toolId: string) => {
    if (!adminKey || !tenantId || !canWriteTools) return;

    try {
      await restoreTool({ adminKey }, tenantId, toolId);
      toast.success("Tool restored", { description: toolId });
      await loadTools();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to restore tool");
    }
  };

  const handleMcpUpdate = async () => {
    if (!adminKey || !tenantId || !canWriteTools) return;

    // In create mode, validate tool_id is provided
    if (mcpCreateMode && !mcpForm.tool_id.trim()) {
      setSchemaError("Tool ID is required");
      return;
    }

    const toolId = mcpCreateMode ? mcpForm.tool_id : selectedTool?.tool_id;
    if (!toolId) return;

    let schemaObj = null;
    if (mcpForm.input_schema_json.trim()) {
      try {
        schemaObj = JSON.parse(mcpForm.input_schema_json);
      } catch (err) {
        setSchemaError("Invalid JSON");
        return;
      }
    }

    setSchemaError("");

    try {
      await updateToolMetadata(
        { adminKey },
        tenantId,
        toolId,
        {
          description: mcpForm.description,
          mcp_upstream_url: mcpForm.upstream_url,
          input_schema_json: schemaObj,
        }
      );
      toast.success(mcpCreateMode ? "MCP connection added" : "MCP tool updated", { description: toolId });
      setMcpDialogOpen(false);
      await loadTools();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update MCP config");
    }
  };

  if (loading) {
    return <ListSkeleton />;
  }

  if (error) {
    return (
      <Card>
        <CardContent className="py-8">
          <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>
        </CardContent>
      </Card>
    );
  }

  if (!tenantId) {
    return <Card><CardContent className="py-8 text-center text-muted-foreground">Select a tenant to view tools.</CardContent></Card>;
  }

  return (
    <>
      <div className="space-y-6">
        {!canReadTools ? (
          <Alert>
            <AlertDescription>Missing scope: {scopeToolsRead}</AlertDescription>
          </Alert>
        ) : null}
        {/* HTTP Tools Section */}
        <Card>
          <CardHeader>
            <div className="flex justify-between items-start">
              <div>
                <CardTitle>HTTP Tools</CardTitle>
                <CardDescription>Traditional HTTP API endpoints with authentication</CardDescription>
              </div>
              <Button variant="default" size="sm" onClick={() => setHttpCreateDialogOpen(true)} disabled={!canWriteTools}>
                Create HTTP Tool
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            {httpTools.length === 0 ? (
              <div className="text-sm text-muted-foreground">No HTTP tools configured.</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Tool ID</TableHead>
                    <TableHead>Base URL</TableHead>
                    <TableHead>Auth Type</TableHead>
                    <TableHead>Auth Value</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {httpTools.map((tool) => {
                    const edit = httpEdits[tool.tool_id] ?? {
                      baseUrl: tool.http?.base_url || "",
                      authType: tool.http?.auth_type || "",
                      authValue: ""
                    };
                    return (
                      <TableRow key={tool.tool_id}>
                        <TableCell className="font-medium">{tool.tool_id}</TableCell>
                        <TableCell>
                          <Input
                            value={edit.baseUrl}
                            onChange={(e) => handleHttpEdit(tool.tool_id, "baseUrl", e.target.value)}
                            disabled={!canWriteTools}
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            value={edit.authType}
                            onChange={(e) => handleHttpEdit(tool.tool_id, "authType", e.target.value)}
                            disabled={!canWriteTools}
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            type="password"
                            placeholder={tool.http?.auth_set ? "••••••" : "Not set"}
                            value={edit.authValue}
                            onChange={(e) => handleHttpEdit(tool.tool_id, "authValue", e.target.value)}
                            disabled={!canWriteTools}
                          />
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex gap-2 justify-end">
                            <Button size="sm" onClick={() => handleHttpUpdate(tool)} disabled={!canWriteTools}>Update</Button>
                            <Button size="sm" variant="destructive" onClick={() => openArchiveDialog(tool.tool_id)} disabled={!canWriteTools}>Archive</Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Separator />

        {/* MCP Tools Section */}
        <Card>
          <CardHeader>
            <div className="flex justify-between items-start">
              <div>
                <CardTitle>MCP Tools</CardTitle>
                <CardDescription>Model Context Protocol endpoints with schema definitions</CardDescription>
              </div>
              <div className="flex gap-2">
                <Button variant="default" size="sm" onClick={openMcpCreateDialog} disabled={!canWriteTools}>Add MCP Connection</Button>
                <Button variant="outline" size="sm" onClick={loadTools} disabled={!canReadTools}>Refresh</Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {mcpTools.length === 0 ? (
              <div className="text-sm text-muted-foreground">
                No MCP tools configured. Click "Add MCP Connection" to create one.
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Tool ID</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead>MCP Upstream URL</TableHead>
                    <TableHead>Schema</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {mcpTools.map((tool) => (
                    <TableRow key={tool.tool_id}>
                      <TableCell className="font-medium">{tool.tool_id}</TableCell>
                      <TableCell className="max-w-xs truncate">
                        {tool.mcp?.description || <span className="text-muted-foreground">—</span>}
                      </TableCell>
                      <TableCell className="max-w-xs truncate font-mono text-xs">
                        {tool.mcp?.upstream_url || <span className="text-muted-foreground">—</span>}
                      </TableCell>
                      <TableCell>
                        {tool.mcp?.input_schema_json ? (
                          <span className="text-xs text-muted-foreground">Configured</span>
                        ) : (
                          <span className="text-xs text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex gap-2 justify-end">
                          <Button size="sm" variant="outline" onClick={() => openMcpEditDialog(tool)} disabled={!canWriteTools}>
                            Edit
                          </Button>
                          <Button size="sm" variant="destructive" onClick={() => openArchiveDialog(tool.tool_id)} disabled={!canWriteTools}>
                            Archive
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

        {/* Archived Tools Section */}
        {archivedTools.length > 0 && (
          <Card>
            <CardHeader>
              <div className="flex justify-between items-start">
                <div>
                  <CardTitle>Archived Tools</CardTitle>
                  <CardDescription>Soft-deleted tools that can be restored</CardDescription>
                </div>
                <Button variant="outline" size="sm" onClick={() => setShowArchived(!showArchived)}>
                  {showArchived ? "Hide" : "Show"} ({archivedTools.length})
                </Button>
              </div>
            </CardHeader>
            {showArchived && (
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Tool ID</TableHead>
                      <TableHead>Type</TableHead>
                      <TableHead>Archived At</TableHead>
                      <TableHead></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {archivedTools.map((tool) => (
                      <TableRow key={tool.tool_id}>
                        <TableCell className="font-medium">{tool.tool_id}</TableCell>
                        <TableCell>{tool.http ? "HTTP" : "MCP"}</TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {tool.archived_at ? new Date(tool.archived_at).toLocaleString() : "—"}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button size="sm" variant="outline" onClick={() => handleRestore(tool.tool_id)} disabled={!canWriteTools}>
                            Restore
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            )}
          </Card>
        )}
      </div>

      {/* Create HTTP Tool Dialog */}
      <Dialog open={httpCreateDialogOpen} onOpenChange={setHttpCreateDialogOpen}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>Create HTTP Tool</DialogTitle>
            <DialogDescription>Configure a new HTTP API endpoint with authentication</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="http_tool_id">Tool ID</Label>
              <Input
                id="http_tool_id"
                placeholder="e.g., stripe, github, custom-api"
                value={httpCreateForm.tool_id}
                onChange={(e) => setHttpCreateForm({ ...httpCreateForm, tool_id: e.target.value })}
                disabled={!canWriteTools}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="http_base_url">Base URL</Label>
              <Input
                id="http_base_url"
                placeholder="https://api.example.com"
                value={httpCreateForm.base_url}
                onChange={(e) => setHttpCreateForm({ ...httpCreateForm, base_url: e.target.value })}
                disabled={!canWriteTools}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="http_auth_type">Auth Type</Label>
              <Input
                id="http_auth_type"
                placeholder="none, bearer, api_key"
                value={httpCreateForm.auth_type}
                onChange={(e) => setHttpCreateForm({ ...httpCreateForm, auth_type: e.target.value })}
                disabled={!canWriteTools}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="http_auth_value">Auth Value</Label>
              <Input
                id="http_auth_value"
                type="password"
                placeholder="API key or bearer token"
                value={httpCreateForm.auth_value}
                onChange={(e) => setHttpCreateForm({ ...httpCreateForm, auth_value: e.target.value })}
                disabled={!canWriteTools}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="http_description">Description</Label>
              <Textarea
                id="http_description"
                placeholder="Brief description of this API"
                value={httpCreateForm.description}
                onChange={(e) => setHttpCreateForm({ ...httpCreateForm, description: e.target.value })}
                rows={2}
                disabled={!canWriteTools}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setHttpCreateDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleHttpCreate} disabled={!canWriteTools}>Create Tool</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Archive Confirmation Dialog */}
      <Dialog open={archiveDialogOpen} onOpenChange={setArchiveDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Archive Tool?</DialogTitle>
            <DialogDescription>
              This will hide "{toolToArchive}" from agents but preserve audit history. You can restore it later from the Archived Tools section.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setArchiveDialogOpen(false)}>Cancel</Button>
            <Button variant="destructive" onClick={handleArchive} disabled={!canWriteTools}>Archive Tool</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* MCP Create/Edit Dialog */}
      <Dialog open={mcpDialogOpen} onOpenChange={setMcpDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {mcpCreateMode ? "Add MCP Connection" : `Edit MCP Tool - ${selectedTool?.tool_id}`}
            </DialogTitle>
            <DialogDescription>
              {mcpCreateMode
                ? "Configure a new MCP tool with upstream URL and schema definition."
                : "Update MCP protocol settings for tool discovery and schema validation."}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {mcpCreateMode && (
              <div className="space-y-2">
                <Label htmlFor="tool_id">Tool ID</Label>
                <Input
                  id="tool_id"
                  placeholder="e.g., github, slack, custom-tool"
                  value={mcpForm.tool_id}
                  onChange={(e) => setMcpForm({ ...mcpForm, tool_id: e.target.value })}
                  disabled={!canWriteTools}
                />
                <p className="text-xs text-muted-foreground">
                  Unique identifier for this MCP tool
                </p>
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                placeholder="Brief description shown in tools/list"
                value={mcpForm.description}
                onChange={(e) => setMcpForm({ ...mcpForm, description: e.target.value })}
                rows={2}
                disabled={!canWriteTools}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="upstream_url">MCP Upstream URL</Label>
              <Input
                id="upstream_url"
                placeholder="https://example.com/mcp"
                value={mcpForm.upstream_url}
                onChange={(e) => setMcpForm({ ...mcpForm, upstream_url: e.target.value })}
                disabled={!canWriteTools}
              />
              <p className="text-xs text-muted-foreground">
                The upstream MCP server endpoint to forward governed requests
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="input_schema_json">Input Schema (JSON Schema)</Label>
              <Textarea
                id="input_schema_json"
                placeholder='{"type":"object","properties":{"action":{"type":"string"}}}'
                value={mcpForm.input_schema_json}
                onChange={(e) => {
                  setMcpForm({ ...mcpForm, input_schema_json: e.target.value });
                  setSchemaError("");
                }}
                rows={10}
                className="font-mono text-xs"
                disabled={!canWriteTools}
              />
              <p className="text-xs text-muted-foreground">
                JSON Schema for tools/call validation. Leave empty for permissive schema.
              </p>
            </div>

            {schemaError && (
              <Alert variant="destructive">
                <AlertDescription>{schemaError}</AlertDescription>
              </Alert>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMcpDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleMcpUpdate} disabled={!canWriteTools}>{mcpCreateMode ? "Add Connection" : "Save"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
