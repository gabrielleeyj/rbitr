import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function PoliciesPage() {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Policy versions</CardTitle>
          <CardDescription>Review and publish active policy versions.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="text-sm text-muted-foreground">
            Policy lifecycle endpoints will populate this list.
          </div>
          <div className="flex gap-2">
            <Button>Publish selected</Button>
            <Button variant="outline">Rollback</Button>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>New policy version</CardTitle>
          <CardDescription>Create a new Rego v1 policy version.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="policy-version">Version</Label>
            <Input id="policy-version" placeholder="p_2026_01_21_001" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="policy-notes">Notes</Label>
            <Input id="policy-notes" placeholder="Reason for change" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="rego">Rego module (rego.v1)</Label>
            <Textarea id="rego" className="min-h-[220px]" placeholder="package rbitr.policy\n\nimport rego.v1\n..." />
          </div>
          <div className="flex gap-2">
            <Button>Save new version</Button>
            <Button variant="outline">Simulate</Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
