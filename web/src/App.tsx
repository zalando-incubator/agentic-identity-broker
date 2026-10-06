import { Suspense, lazy, useState } from 'react';
import { createBrowserRouter, createRoutesFromElements, Navigate, Outlet, Route, RouterProvider } from 'react-router-dom';
import { GlobalErrorBoundary } from '@design-system/components/feedback/GlobalErrorBoundary/GlobalErrorBoundary';
import { Skeleton } from '@design-system/components/feedback/Skeleton/Skeleton';
import { DecisionShell } from '@design-system/components/layout/DecisionShell/DecisionShell';
import AgentRoute from '@components/layout/AgentRoute';
import { commonCopy, navigationCopy } from '@copy';

// Console-only search and motion stay out of initial decision downloads.
const ConsoleLayout = lazy(() => import('@components/layout/ConsoleLayout'));
const DelegationsPage = lazy(() => import('./pages/DelegationsPage'));
const ConnectionsPage = lazy(() => import('./pages/ConnectionsPage').then(({ ConnectionsPage }) => ({ default: ConnectionsPage })));
const ApprovalPage = lazy(() => import('./pages/ApprovalPage'));
const ApprovalsPage = lazy(() => import('./pages/ApprovalsPage'));
const SettingsPage = lazy(() => import('./pages/SettingsPage'));
const ErrorPage = lazy(() => import('./pages/ErrorPage'));

function LoadingFallback() {
  return <div className="mx-auto w-full max-w-2xl p-6"><Skeleton label={commonCopy.loading} count={3} /></div>;
}

export default function App() {
  const [router] = useState(() => createBrowserRouter(createRoutesFromElements(
    <Route element={<Suspense fallback={<LoadingFallback />}><Outlet /></Suspense>}>
      <Route path="/" element={<Navigate to="/agents" replace />} />
      <Route element={<ConsoleLayout />}>
        <Route path="/agents" element={<DelegationsPage />} />
        <Route path="/connections" element={<ConnectionsPage />} />
        <Route path="/approvals" element={<ApprovalsPage />}>
          <Route path="remembered" />
        </Route>
        <Route path="/settings/appearance" element={<SettingsPage />} />
        <Route path="*" element={<ErrorPage />} />
      </Route>
      <Route path="/agents/:agentId" element={<AgentRoute />} />
      <Route path="/approvals/:id" element={
        <DecisionShell wordmarkLabel={commonCopy.brand} skipToMainLabel={navigationCopy.skipToContent}>
          <ApprovalPage />
        </DecisionShell>
      } />
    </Route>,
  )));
  return <GlobalErrorBoundary title={commonCopy.errorTitle} description={commonCopy.errorMessage} retryLabel={commonCopy.retry}>
    <RouterProvider router={router} />
  </GlobalErrorBoundary>;
}
