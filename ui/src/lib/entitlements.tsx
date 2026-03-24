import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { useAdminKey } from "@/lib/auth";
import { getEntitlements, type EntitlementsResponse } from "@/lib/api";

interface EntitlementsContextValue {
  entitlements: EntitlementsResponse | null;
  loading: boolean;
  hasFeature: (feature: string) => boolean;
  refresh: () => void;
}

const EntitlementsContext = createContext<EntitlementsContextValue>({
  entitlements: null,
  loading: true,
  hasFeature: () => false,
  refresh: () => {},
});

export function EntitlementsProvider({ children }: { children: React.ReactNode }) {
  const { adminKey } = useAdminKey();
  const [entitlements, setEntitlements] = useState<EntitlementsResponse | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(() => {
    if (!adminKey) {
      setLoading(false);
      return;
    }
    setLoading(true);
    getEntitlements({ adminKey })
      .then((data) => {
        setEntitlements(data);
      })
      .catch(() => {
        // Fall back to null — features will show as locked.
      })
      .finally(() => setLoading(false));
  }, [adminKey]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const hasFeature = useCallback(
    (feature: string): boolean => {
      if (!entitlements) return false;
      const features = entitlements.features as Record<string, boolean>;
      return features[feature] ?? false;
    },
    [entitlements]
  );

  return (
    <EntitlementsContext.Provider value={{ entitlements, loading, hasFeature, refresh }}>
      {children}
    </EntitlementsContext.Provider>
  );
}

export function useEntitlements() {
  return useContext(EntitlementsContext);
}
