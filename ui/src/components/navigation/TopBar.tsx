import { useEffect } from "react";
import { Link, useLocation } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useTenant } from "@/lib/tenant";

const titles: Record<string, string> = {
  "/tenants": "Tenants",
  "/evidence": "Evidence",
  "/approvals": "Approvals",
  "/policies": "Policies",
  "/risk-overrides": "Risk Overrides",
  "/tools": "Tools",
  "/settings": "Settings",
  "/notifications": "Notifications",
  "/ticketing": "Ticketing",
  "/license": "License",
  "/usage": "Usage",
  "/audit": "Audit Log",
};

function pageTitle(pathname: string): string {
  const exact = titles[pathname];
  if (exact) return exact;
  if (pathname.startsWith("/approvals/")) return "Approval Request";
  return "Control Plane";
}

export function TopBar() {
  const { pathname } = useLocation();
  const { selectedTenant } = useTenant();

  const title = pageTitle(pathname);

  useEffect(() => {
    document.title = `${title} · rbitr`;
  }, [title]);

  return (
    <div className="flex flex-wrap items-center justify-between gap-4 px-4 py-4 md:px-6">
      <div>
        <div className="text-xs uppercase tracking-widest text-muted-foreground">
          rbitr control plane
        </div>
        <h1 className="font-display text-3xl font-bold tracking-tight">{title}</h1>
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
