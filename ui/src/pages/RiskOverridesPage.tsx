import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function RiskOverridesPage() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Risk overrides</CardTitle>
        <CardDescription>Upsert or remove action risk overrides.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="action-type">Action type</Label>
            <Input id="action-type" placeholder="DATA.EXPORT" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="risk">Risk</Label>
            <Input id="risk" placeholder="HIGH" />
          </div>
          <div className="flex items-end gap-2">
            <Button>Upsert</Button>
            <Button variant="outline">Delete</Button>
          </div>
        </div>
        <div className="text-sm text-muted-foreground">Overrides list will appear here.</div>
      </CardContent>
    </Card>
  );
}
