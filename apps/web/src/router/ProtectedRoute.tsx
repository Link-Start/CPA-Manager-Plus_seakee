import { useEffect, useState, type ReactElement } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuthStore } from '@/stores';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { isUsageServiceId, usageServiceApi } from '@/services/api/usageService';
import { detectApiBaseFromLocation } from '@/utils/connection';

export function ProtectedRoute({ children }: { children: ReactElement }) {
  const location = useLocation();
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const managementKey = useAuthStore((state) => state.managementKey);
  const apiBase = useAuthStore((state) => state.apiBase);
  const restoreSession = useAuthStore((state) => state.restoreSession);
  const [checking, setChecking] = useState(false);

  useEffect(() => {
    const tryRestore = async () => {
      if (!isAuthenticated && managementKey && apiBase) {
        setChecking(true);
        try {
          const detectedBase = detectApiBaseFromLocation();
          let detectedUsageService = false;
          try {
            const info = await usageServiceApi.getInfo(detectedBase);
            detectedUsageService = isUsageServiceId(info.service);
          } catch {
            detectedUsageService = false;
          }
          const hostedManagementPage =
            typeof window !== 'undefined' &&
            /\/management\.html$/i.test(window.location.pathname);
          await restoreSession({
            expectedMode: detectedUsageService ? 'manager_embedded' : 'external_panel',
            expectedPanelBase:
              detectedUsageService || hostedManagementPage ? detectedBase : undefined,
          });
        } finally {
          setChecking(false);
        }
      }
    };
    tryRestore();
  }, [apiBase, isAuthenticated, managementKey, restoreSession]);

  if (checking) {
    return (
      <div className="main-content">
        <LoadingSpinner />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return children;
}
