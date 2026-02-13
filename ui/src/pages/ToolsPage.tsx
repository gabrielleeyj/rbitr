import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { listTools, updateTool, updateToolMetadata, type ToolConfig } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { useTenant } from "@/lib/tenant";
import { toast } from "sonner";

export function ToolsPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant } = useTenant();
  const tenantId = selectedTenant?.tenant_id;
  const [tools, setTools] = useState<ToolConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // HTTP tool editing
  const [httpEdits, setHttpEdits] = useState<Record<string, { baseUrl: string; authType: string; authValue: string }>>({});

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

  useEffect(() => {
    loadTools();
  }, [adminKey, tenantId]);

  const loadTools = async () => {
    if (!adminKey || !tenantId) {
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

  const httpTools = tools.filter(t => t.http);
  // Show only tools with MCP configuration that have an upstream URL configured
  const mcpTools = tools.filter(t => t.mcp && t.mcp.upstream_url && t.mcp.upstream_url.trim() !== "");

  const handleHttpEdit = (toolId: string, field: "baseUrl" | "authType" | "authValue", value: string) => {
    setHttpEdits((prev) => ({
      ...prev,
      [toolId]: { ...(prev[toolId] ?? { baseUrl: "", authType: "", authValue: "" }), [field]: value },
    }));
  };

  const handleHttpUpdate = async (tool: ToolConfig) => {
    if (!adminKey || !tenantId) return;
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

  const handleMcpUpdate = async () => {
    if (!adminKey || !tenantId) return;

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
    return <Card><CardContent className="py-8 text-center text-muted-foreground">Loading tools...</CardContent></Card>;
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
        {/* HTTP Tools Section */}
        <Card>
          <CardHeader>
            <CardTitle>HTTP Tools</CardTitle>
            <CardDescription>Traditional HTTP API endpoints with authentication</CardDescription>
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
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            value={edit.authType}
                            onChange={(e) => handleHttpEdit(tool.tool_id, "authType", e.target.value)}
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            type="password"
                            placeholder={tool.http?.auth_set ? "••••••" : "Not set"}
                            value={edit.authValue}
                            onChange={(e) => handleHttpEdit(tool.tool_id, "authValue", e.target.value)}
                          />
                        </TableCell>
                        <TableCell className="text-right">
                          <Button size="sm" onClick={() => handleHttpUpdate(tool)}>Update</Button>
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
                <Button variant="default" size="sm" onClick={openMcpCreateDialog}>Add MCP Connection</Button>
                <Button variant="outline" size="sm" onClick={loadTools}>Refresh</Button>
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
                        <Button size="sm" variant="outline" onClick={() => openMcpEditDialog(tool)}>
                          Edit
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

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
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="upstream_url">MCP Upstream URL</Label>
              <Input
                id="upstream_url"
                placeholder="https://example.com/mcp"
                value={mcpForm.upstream_url}
                onChange={(e) => setMcpForm({ ...mcpForm, upstream_url: e.target.value })}
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
            <Button onClick={handleMcpUpdate}>{mcpCreateMode ? "Add Connection" : "Save"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
