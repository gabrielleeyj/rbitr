import { createContext, useContext, useMemo, useState } from "react";
import { Navigate } from "react-router-dom";

const storageKey = "rbitr_admin_key";

interface AdminKeyContextValue {
  adminKey: string | null;
  setAdminKey: (value: string) => void;
  clearAdminKey: () => void;
}

const AdminKeyContext = createContext<AdminKeyContextValue | undefined>(undefined);

export function AdminKeyProvider({ children }: { children: React.ReactNode }) {
  const [adminKey, setAdminKeyState] = useState<string | null>(() => {
    return localStorage.getItem(storageKey);
  });

  const setAdminKey = (value: string) => {
    localStorage.setItem(storageKey, value);
    setAdminKeyState(value);
  };

  const clearAdminKey = () => {
    localStorage.removeItem(storageKey);
    setAdminKeyState(null);
  };

  const value = useMemo(
    () => ({ adminKey, setAdminKey, clearAdminKey }),
    [adminKey]
  );

  return <AdminKeyContext.Provider value={value}>{children}</AdminKeyContext.Provider>;
}

export function useAdminKey() {
  const context = useContext(AdminKeyContext);
  if (!context) {
    throw new Error("useAdminKey must be used within AdminKeyProvider");
  }
  return context;
}

export function RequireAdminKey({ children }: { children: React.ReactNode }) {
  const { adminKey } = useAdminKey();
  if (!adminKey) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}
