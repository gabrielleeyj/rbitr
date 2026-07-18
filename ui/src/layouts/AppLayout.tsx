import { Outlet } from "react-router-dom";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarProvider,
} from "@/components/ui/sidebar";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { Separator } from "@/components/ui/separator";
import { AppNav } from "@/components/navigation/AppNav";
import { TopBar } from "@/components/navigation/TopBar";
import { GatewayLogo } from "@/components/gateway-logo";
import { Toaster } from "@/components/ui/sonner";
import { useAdminKey } from "@/lib/auth";

export function AppLayout() {
  const { clearAdminKey } = useAdminKey();

  return (
    <SidebarProvider defaultOpen>
      <Sidebar>
        <SidebarHeader className="border-b border-sidebar-border">
          <div className="flex items-center gap-3 px-4 py-4">
            <GatewayLogo className="h-10 w-10" />
            <div>
              <div className="font-display text-lg font-bold tracking-tight">
                rbitr
              </div>
              <div className="text-xs text-muted-foreground">control plane</div>
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <AppNav />
        </SidebarContent>
        <SidebarFooter className="border-t border-sidebar-border">
          <div className="flex flex-col gap-2 p-3">
            <Button variant="outline" size="sm" onClick={clearAdminKey}>
              End admin session
            </Button>
            <ThemeToggle />
          </div>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <TopBar />
        <Separator />
        <main className="px-4 py-4 md:px-6 md:py-6">
          <Outlet />
        </main>
      </SidebarInset>
      <Toaster richColors position="top-right" />
    </SidebarProvider>
  );
}
