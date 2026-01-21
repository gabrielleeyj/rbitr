import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { listTenants } from "@/lib/api";
import { useAdminKey } from "@/lib/auth";
import { TenantSummary, useTenant } from "@/lib/tenant";

export function TenantsPage() {
  const { adminKey } = useAdminKey();
  const { selectedTenant, setSelectedTenant } = useTenant();
  const [tenants, setTenants] = useState<TenantSummary[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    const load = async () => {
      if (!adminKey) {
        return;
      }
      try {
        const data = await listTenants({ adminKey });
        if (isMounted) {
          setTenants(data);
          setLoading(false);
        }
      } catch (err) {
        if (isMounted) {
          setError(err instanceof Error ? err.message : "Failed to load tenants.");
          setLoading(false);
        }
      }
    };

    load();

    return () => {
      isMounted = false;
    };
  }, [adminKey]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tenants</CardTitle>
        <CardDescription>Select a tenant to manage policies and evidence.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading tenants...</div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Tenant</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Active policy</TableHead>
                <TableHead>Tools</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tenants.map((tenant) => (
                <TableRow key={tenant.tenant_id}>
                  <TableCell className="font-medium">{tenant.tenant_id}</TableCell>
                  <TableCell>{tenant.name ?? "—"}</TableCell>
                  <TableCell>{tenant.active_policy_version ?? "—"}</TableCell>
                  <TableCell>{tenant.tool_count ?? "—"}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant={selectedTenant?.tenant_id === tenant.tenant_id ? "secondary" : "outline"}
                      onClick={() => setSelectedTenant(tenant)}
                    >
                      {selectedTenant?.tenant_id === tenant.tenant_id ? "Selected" : "Select"}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
