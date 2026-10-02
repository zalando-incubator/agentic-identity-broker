import { Suspense, lazy } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { GlobalErrorBoundary } from '@design-system/components/feedback/GlobalErrorBoundary/GlobalErrorBoundary';
import { Skeleton } from '@design-system/components/feedback/Skeleton/Skeleton';
import { DecisionShell } from '@design-system/components/layout/DecisionShell/DecisionShell';
import AgentRoute from '@components/layout/AgentRoute';
import { commonCopy, navigationCopy } from '@copy';

// Route-level loading keeps console tables and command search out of decision downloads.
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
  return (
    <GlobalErrorBoundary title={commonCopy.errorTitle} description={commonCopy.errorMessage} retryLabel={commonCopy.retry}>
      <BrowserRouter>
        <Suspense fallback={<LoadingFallback />}>
          <Routes>
            <Route path="/" element={<Navigate to="/delegations" replace />} />
            <Route element={<ConsoleLayout />}>
              <Route path="/delegations" element={<DelegationsPage />} />
              <Route path="/sessions" element={<ConnectionsPage />} />
              <Route path="/approvals" element={<ApprovalsPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="*" element={<ErrorPage />} />
            </Route>
            <Route path="/agents/:agentId" element={<AgentRoute />} />
            <Route path="/approvals/:id" element={
              <DecisionShell wordmarkLabel={commonCopy.brand} skipToMainLabel={navigationCopy.skipToContent} footerLabel={commonCopy.poweredBy}>
                <ApprovalPage />
              </DecisionShell>
            } />
          </Routes>
        </Suspense>
      </BrowserRouter>
    </GlobalErrorBoundary>
  );
}
