import { createContext, useContext, useMemo, useState } from "react";

export interface TenantSummary {
  tenant_id: string;
  name?: string;
  active_policy_version?: string;
  tool_count?: number;
}

const storageKey = "rbitr_selected_tenant";

interface TenantContextValue {
  selectedTenant: TenantSummary | null;
  setSelectedTenant: (tenant: TenantSummary) => void;
}

const TenantContext = createContext<TenantContextValue | undefined>(undefined);

export function TenantProvider({ children }: { children: React.ReactNode }) {
  const [selectedTenant, setSelectedTenantState] = useState<TenantSummary | null>(() => {
    const stored = localStorage.getItem(storageKey);
    return stored ? (JSON.parse(stored) as TenantSummary) : null;
  });

  const setSelectedTenant = (tenant: TenantSummary) => {
    localStorage.setItem(storageKey, JSON.stringify(tenant));
    setSelectedTenantState(tenant);
  };

  const value = useMemo(
    () => ({ selectedTenant, setSelectedTenant }),
    [selectedTenant]
  );

  return <TenantContext.Provider value={value}>{children}</TenantContext.Provider>;
}

export function useTenant() {
  const context = useContext(TenantContext);
  if (!context) {
    throw new Error("useTenant must be used within TenantProvider");
  }
  return context;
}
