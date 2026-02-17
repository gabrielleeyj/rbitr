import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { NavLink } from "react-router-dom";
import {
  Building2,
  CheckCircle2,
  FileSearch,
  ShieldCheck,
  ShieldAlert,
  Wrench,
  Settings,
  BellRing,
  ClipboardList,
} from "lucide-react";
import { useTenant } from "@/lib/tenant";
import { useAdminKey } from "@/lib/auth";
import {
  scopeApprovalsRead,
  scopeAuditRead,
  scopeNotificationsRead,
  scopePoliciesRead,
  scopeSettingsRead,
  scopeTenantsRead,
  scopeToolsRead,
} from "@/lib/scopes";

const navItems = [
  { to: "/tenants", label: "Tenants", icon: Building2, scope: scopeTenantsRead },
  { to: "/evidence", label: "Evidence", icon: FileSearch, scope: scopeAuditRead },
  { to: "/approvals", label: "Approvals", icon: CheckCircle2, scope: scopeApprovalsRead },
  { to: "/policies", label: "Policies", icon: ShieldCheck, scope: scopePoliciesRead },
  { to: "/risk-overrides", label: "Risk Overrides", icon: ShieldAlert, scope: scopePoliciesRead },
  { to: "/tools", label: "Tools", icon: Wrench, scope: scopeToolsRead },
  { to: "/settings", label: "Settings", icon: Settings, scope: scopeSettingsRead },
  { to: "/notifications", label: "Notifications", icon: BellRing, scope: scopeNotificationsRead },
  { to: "/audit", label: "Audit", icon: ClipboardList, scope: scopeAuditRead },
];

export function AppNav() {
  const { selectedTenant } = useTenant();
  const { hasScope, scopesLoading } = useAdminKey();
  const visibleItems = navItems.filter((item) => hasScope(item.scope));

  return (
    <SidebarGroup>
      <SidebarGroupLabel>Control Plane</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {visibleItems.map((item) => {
            const Icon = item.icon;
            return (
              <SidebarMenuItem key={item.to}>
                <SidebarMenuButton asChild tooltip={item.label}>
                  <NavLink
                    to={item.to}
                    className={({ isActive }) =>
                      isActive
                        ? "bg-sidebar-accent text-sidebar-accent-foreground"
                        : "text-sidebar-foreground/80"
                    }
                  >
                    <Icon className="h-4 w-4" />
                    <span>{item.label}</span>
                    {item.to === "/evidence" && selectedTenant ? (
                      <SidebarMenuBadge>{selectedTenant.tenant_id}</SidebarMenuBadge>
                    ) : null}
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            );
          })}
          {!scopesLoading && visibleItems.length === 0 ? (
            <SidebarMenuItem>
              <div className="px-2 py-1 text-xs text-sidebar-foreground/70">
                No pages available for current scopes.
              </div>
            </SidebarMenuItem>
          ) : null}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
