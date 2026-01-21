import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ToolsPage() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Tools</CardTitle>
        <CardDescription>Manage tool connectors and base URLs.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="tool-id">Tool ID</Label>
            <Input id="tool-id" placeholder="mock_internal" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="base-url">Base URL</Label>
            <Input id="base-url" placeholder="http://localhost:8090" />
          </div>
          <div className="flex items-end gap-2">
            <Button>Update</Button>
            <Button variant="outline">Disable</Button>
          </div>
        </div>
        <div className="text-sm text-muted-foreground">Tool list will appear here.</div>
      </CardContent>
    </Card>
  );
}
