import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { AdminKeyProvider, RequireAdminKey } from "@/lib/auth";
import { TenantProvider } from "@/lib/tenant";
import { getSetupStatus, type SetupStatus } from "@/lib/api";
import { AppLayout } from "@/layouts/AppLayout";
import { LoginPage } from "@/pages/LoginPage";
import { TenantsPage } from "@/pages/TenantsPage";
import { EvidencePage } from "@/pages/EvidencePage";
import { PoliciesPage } from "@/pages/PoliciesPage";
import { RiskOverridesPage } from "@/pages/RiskOverridesPage";
import { ToolsPage } from "@/pages/ToolsPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { AuditPage } from "@/pages/AuditPage";
import { ApprovalsPage } from "@/pages/ApprovalsPage";
import { ApprovalDetailPage } from "@/pages/ApprovalDetailPage";
import { NotificationsPage } from "@/pages/NotificationsPage";
import { SetupPage } from "@/pages/SetupPage";

export function App() {
  return (
    <AdminKeyProvider>
      <TenantProvider>
        <AppRoutes />
      </TenantProvider>
    </AdminKeyProvider>
  );
}

function AppRoutes() {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refreshStatus = async () => {
    setLoading(true);
    setError("");
    try {
      const setupStatus = await getSetupStatus();
      setStatus(setupStatus);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load setup status.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refreshStatus();
  }, []);

  if (loading) {
    return (
      <div className="min-h-screen bg-muted/30 flex items-center justify-center px-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Loading rbitr</CardTitle>
            <CardDescription>Checking first-run setup status.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="text-sm text-muted-foreground">Please wait...</div>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-muted/30 flex items-center justify-center px-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Unable to load setup status</CardTitle>
            <CardDescription>Cannot determine whether onboarding is required.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
            <Button onClick={() => void refreshStatus()}>Retry</Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!status) {
    return (
      <div className="min-h-screen bg-muted/30 flex items-center justify-center px-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Setup status unavailable</CardTitle>
            <CardDescription>Retry to continue.</CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={() => void refreshStatus()}>Retry</Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const setupRequired = status.setup_required;

  return (
    <Routes>
      <Route
        path="/setup"
        element={
          setupRequired ? (
            <SetupPage
              status={status}
              onRefreshStatus={refreshStatus}
              onSetupCompleted={refreshStatus}
            />
          ) : (
            <Navigate to="/" replace />
          )
        }
      />
      <Route
        path="/login"
        element={setupRequired ? <Navigate to="/setup" replace /> : <LoginPage />}
      />
      <Route
        path="/"
        element={
          setupRequired ? (
            <Navigate to="/setup" replace />
          ) : (
            <RequireAdminKey>
              <AppLayout />
            </RequireAdminKey>
          )
        }
      >
        <Route index element={<Navigate to="/tenants" replace />} />
        <Route path="tenants" element={<TenantsPage />} />
        <Route path="evidence" element={<EvidencePage />} />
        <Route path="policies" element={<PoliciesPage />} />
        <Route path="risk-overrides" element={<RiskOverridesPage />} />
        <Route path="approvals" element={<ApprovalsPage />} />
        <Route path="approvals/:approvalId" element={<ApprovalDetailPage />} />
        <Route path="tools" element={<ToolsPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="notifications" element={<NotificationsPage />} />
        <Route path="audit" element={<AuditPage />} />
      </Route>
      <Route path="*" element={<Navigate to={setupRequired ? "/setup" : "/"} replace />} />
    </Routes>
  );
}
