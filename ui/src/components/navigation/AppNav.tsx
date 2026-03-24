import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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
  Ticket,
  ClipboardList,
  KeyRound,
  BarChart3,
  Lock,
} from "lucide-react";
import { useTenant } from "@/lib/tenant";
import { useAdminKey } from "@/lib/auth";
import { useEntitlements } from "@/lib/entitlements";
import {
  scopeApprovalsRead,
  scopeAuditRead,
  scopeLicenseRead,
  scopeNotificationsRead,
  scopePoliciesRead,
  scopeSettingsRead,
  scopeTenantsRead,
  scopeTicketingRead,
  scopeToolsRead,
} from "@/lib/scopes";

interface NavItem {
  to: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  scope: string;
  /** If set, this nav item is gated behind the named entitlement feature. */
  gatedFeature?: string;
}

const navItems: NavItem[] = [
  { to: "/tenants", label: "Tenants", icon: Building2, scope: scopeTenantsRead },
  { to: "/evidence", label: "Evidence", icon: FileSearch, scope: scopeAuditRead },
  { to: "/approvals", label: "Approvals", icon: CheckCircle2, scope: scopeApprovalsRead, gatedFeature: "approval_workflows" },
  { to: "/policies", label: "Policies", icon: ShieldCheck, scope: scopePoliciesRead },
  { to: "/risk-overrides", label: "Risk Overrides", icon: ShieldAlert, scope: scopePoliciesRead },
  { to: "/tools", label: "Tools", icon: Wrench, scope: scopeToolsRead },
  { to: "/settings", label: "Settings", icon: Settings, scope: scopeSettingsRead },
  { to: "/notifications", label: "Notifications", icon: BellRing, scope: scopeNotificationsRead, gatedFeature: "integrations" },
  { to: "/ticketing", label: "Ticketing", icon: Ticket, scope: scopeTicketingRead, gatedFeature: "integrations" },
  { to: "/license", label: "License", icon: KeyRound, scope: scopeLicenseRead },
  { to: "/usage", label: "Usage", icon: BarChart3, scope: scopeLicenseRead },
  { to: "/audit", label: "Audit", icon: ClipboardList, scope: scopeAuditRead },
];

export function AppNav() {
  const { selectedTenant } = useTenant();
  const { hasScope, scopesLoading } = useAdminKey();
  const { hasFeature } = useEntitlements();
  const visibleItems = navItems.filter((item) => hasScope(item.scope));

  return (
    <SidebarGroup>
      <SidebarGroupLabel>Control Plane</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {visibleItems.map((item) => {
            const Icon = item.icon;
            const isLocked = item.gatedFeature ? !hasFeature(item.gatedFeature) : false;

            if (isLocked) {
              return (
                <SidebarMenuItem key={item.to}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <SidebarMenuButton asChild tooltip={item.label}>
                        <NavLink
                          to={item.to}
                          className="text-sidebar-foreground/40"
                        >
                          <Icon className="h-4 w-4" />
                          <span>{item.label}</span>
                          <Lock className="ml-auto h-3 w-3" />
                        </NavLink>
                      </SidebarMenuButton>
                    </TooltipTrigger>
                    <TooltipContent side="right">
                      <p className="text-xs">
                        Upgrade to unlock — upload a license key in License settings
                      </p>
                    </TooltipContent>
                  </Tooltip>
                </SidebarMenuItem>
              );
            }

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
