import { Link, useLocation } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useTenant } from "@/lib/tenant";

const titles: Record<string, string> = {
  "/tenants": "Tenants",
  "/evidence": "Evidence",
  "/policies": "Policies",
  "/risk-overrides": "Risk Overrides",
  "/tools": "Tools",
  "/settings": "Settings",
  "/audit": "Audit Log",
};

export function TopBar() {
  const { pathname } = useLocation();
  const { selectedTenant } = useTenant();

  const title = titles[pathname] ?? "Control Plane";

  return (
    <div className="flex flex-wrap items-center justify-between gap-4 px-6 py-4">
      <div>
        <div className="text-sm text-muted-foreground">rbitr control plane</div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      </div>
      <div className="flex items-center gap-3">
        {selectedTenant ? (
          <Badge variant="secondary">Tenant: {selectedTenant.tenant_id}</Badge>
        ) : (
          <Badge variant="outline">No tenant selected</Badge>
        )}
        <Button variant="outline" size="sm" asChild>
          <Link to="/tenants">Change tenant</Link>
        </Button>
      </div>
    </div>
  );
}
