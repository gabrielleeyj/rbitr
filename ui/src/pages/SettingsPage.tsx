import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";

export function SettingsPage() {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Admin write lock</CardTitle>
          <CardDescription>Freeze all admin writes across tenants.</CardDescription>
        </CardHeader>
        <CardContent className="flex items-center justify-between">
          <div>
            <Label htmlFor="write-lock" className="text-sm">Write lock enabled</Label>
          </div>
          <Switch id="write-lock" />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Audit trail</CardTitle>
          <CardDescription>Recent admin changes appear in the Audit tab.</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline">View audit log</Button>
        </CardContent>
      </Card>
    </div>
  );
}
