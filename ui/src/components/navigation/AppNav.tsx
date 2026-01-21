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
  FileSearch,
  ShieldCheck,
  ShieldAlert,
  Wrench,
  Settings,
  ClipboardList,
} from "lucide-react";
import { useTenant } from "@/lib/tenant";

const navItems = [
  { to: "/tenants", label: "Tenants", icon: Building2 },
  { to: "/evidence", label: "Evidence", icon: FileSearch },
  { to: "/policies", label: "Policies", icon: ShieldCheck },
  { to: "/risk-overrides", label: "Risk Overrides", icon: ShieldAlert },
  { to: "/tools", label: "Tools", icon: Wrench },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/audit", label: "Audit", icon: ClipboardList },
];

export function AppNav() {
  const { selectedTenant } = useTenant();

  return (
    <SidebarGroup>
      <SidebarGroupLabel>Control Plane</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {navItems.map((item) => {
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
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
