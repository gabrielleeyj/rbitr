import { Navigate, Route, Routes } from "react-router-dom";

import { AdminKeyProvider, RequireAdminKey } from "@/lib/auth";
import { TenantProvider } from "@/lib/tenant";
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

export function App() {
  return (
    <AdminKeyProvider>
      <TenantProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <RequireAdminKey>
                <AppLayout />
              </RequireAdminKey>
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
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </TenantProvider>
    </AdminKeyProvider>
  );
}
