import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { Navigate } from "react-router-dom";
import { getAdminMe } from "@/lib/api";
import { hasAdminScope } from "@/lib/scopes";

const storageKey = "rbitr_admin_key";

interface AdminKeyContextValue {
  adminKey: string | null;
  scopes: string[];
  scopesLoading: boolean;
  setAdminKey: (value: string) => void;
  clearAdminKey: () => void;
  hasScope: (scope: string) => boolean;
}

const AdminKeyContext = createContext<AdminKeyContextValue | undefined>(undefined);

export function AdminKeyProvider({ children }: { children: React.ReactNode }) {
  const [adminKey, setAdminKeyState] = useState<string | null>(() => {
    return localStorage.getItem(storageKey);
  });
  const [scopes, setScopes] = useState<string[]>([]);
  const [scopesLoading, setScopesLoading] = useState<boolean>(() => Boolean(localStorage.getItem(storageKey)));

  const setAdminKey = (value: string) => {
    localStorage.setItem(storageKey, value);
    setAdminKeyState(value);
    setScopes([]);
    setScopesLoading(true);
  };

  const clearAdminKey = () => {
    localStorage.removeItem(storageKey);
    setAdminKeyState(null);
    setScopes([]);
    setScopesLoading(false);
  };

  useEffect(() => {
    let mounted = true;
    const loadScopes = async () => {
      if (!adminKey) {
        if (mounted) {
          setScopes([]);
          setScopesLoading(false);
        }
        return;
      }
      setScopesLoading(true);
      try {
        const data = await getAdminMe({ adminKey });
        if (!mounted) return;
        setScopes(data.scopes ?? []);
      } catch {
        if (!mounted) return;
        setScopes([]);
      } finally {
        if (mounted) {
          setScopesLoading(false);
        }
      }
    };
    loadScopes();
    return () => {
      mounted = false;
    };
  }, [adminKey]);

  const hasScope = useCallback(
    (scope: string) => hasAdminScope(scopes, scope),
    [scopes]
  );

  const value = useMemo(
    () => ({ adminKey, scopes, scopesLoading, setAdminKey, clearAdminKey, hasScope }),
    [adminKey, scopes, scopesLoading, hasScope]
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
