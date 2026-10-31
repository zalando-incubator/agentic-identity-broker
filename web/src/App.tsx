/**
 * Main App component with React Router setup.
 *
 * Configures routes for the consent management application:
 * - / -> Redirect to the consent overview
 * - /delegations -> Consent overview (list of agent delegations)
 * - /agents/:agentId -> Agent grant detail page
 * - /sessions -> Third-party OAuth2 sessions management
 * - /sessions/:service -> One service's session page (Virtual Service Scope URL)
 * - * -> 404 error page
 *
 * Performance Optimizations:
 * - Code splitting with React.lazy() for routes
 * - Suspense with loading fallback
 * - Global error boundary
 * - Toast provider for notifications
 */

import { Suspense, lazy } from 'react';
import { BrowserRouter as Router, Navigate, Routes, Route } from 'react-router-dom';
import { GlobalErrorBoundary } from '@components/ui/GlobalErrorBoundary';
import { ToastProvider } from '@components/ui/Toast';

// Code-split route components using React.lazy()
// This creates separate bundles for each route, reducing initial load time
const ConsentOverviewPage = lazy(() => import('./pages/ConsentOverviewPage'));
const AgentGrantDetailPage = lazy(() => import('./pages/AgentGrantDetailPage'));
const ThirdPartySessionsPage = lazy(
  () => import('./pages/ThirdPartySessionsPage'),
);
const ThirdPartySessionDetailPage = lazy(
  () => import('./pages/ThirdPartySessionDetailPage'),
);
const ApprovalPage = lazy(() => import('./pages/ApprovalPage'));
const ToolAuthorizationsPage = lazy(() => import('./pages/ToolAuthorizationsPage'));
const ErrorPage = lazy(() => import('./pages/ErrorPage'));

/**
 * Loading fallback component shown during code splitting lazy load.
 */
function LoadingFallback() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-neutral-50">
      <div className="text-center">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent"></div>
        <p className="mt-4 text-neutral-600">Loading...</p>
      </div>
    </div>
  );
}

function App() {
  return (
    <GlobalErrorBoundary>
      <ToastProvider>
        <Router basename="/">
          <Suspense fallback={<LoadingFallback />}>
            <Routes>
              <Route path="/" element={<Navigate to="/delegations" replace />} />
              <Route path="/delegations" element={<ConsentOverviewPage />} />
              <Route
                path="/agents/:agentId"
                element={<AgentGrantDetailPage />}
              />
              <Route
                path="/sessions"
                element={<ThirdPartySessionsPage />}
              />
              <Route
                path="/sessions/:service"
                element={<ThirdPartySessionDetailPage />}
              />
              <Route
                path="/approvals/:id"
                element={<ApprovalPage />}
              />
              <Route
                path="/approvals"
                element={<ToolAuthorizationsPage />}
              />
              <Route path="*" element={<ErrorPage />} />
            </Routes>
          </Suspense>
        </Router>
      </ToastProvider>
    </GlobalErrorBoundary>
  );
}

export default App;
