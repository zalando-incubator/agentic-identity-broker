/**
 * ThirdPartySessionsPage
 *
 * Displays all OAuth2 sessions for the current user with management capabilities.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Grid layout of session cards
 * - Loading state with skeletons
 * - Error state with retry option
 * - Empty state when no sessions exist
 * - Confirmation dialog before termination
 * - Real-time status updates
 * - WCAG 2.1 AA compliant
 */

import React, { useState, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { AppLayout } from '@components/layout/AppLayout';
import { Grid } from '@design-system/components/layout/Grid';
import { EmptyState } from '@design-system/components/feedback/EmptyState';
import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { Alert } from '@design-system/components/feedback/Alert';
import { Button } from '@design-system/components/primitives/Button';
import { PageTransition } from '@components/ui/PageTransition';
import { useSessions } from '@hooks/useSessions';
import { SessionCard } from '@components/sessions/SessionCard';
import { TerminationDialog } from '@components/sessions/TerminationDialog';
import { sessionsApi } from '@services/api/sessions';
import { extractApiError } from '@utils/api';
import type { SessionDetail } from '@services/api/sessions';

/**
 * Alert state for OAuth2 callback success/error messages
 */
interface AlertState {
  type: 'success' | 'error';
  message: string;
}

/**
 * ThirdPartySessionsPage displays all OAuth2 sessions for the current user.
 *
 * Users can:
 * - View all their active sessions with status information
 * - See which agents depend on each session
 * - Terminate sessions (with confirmation warning)
 * - Re-authenticate if a session has expired
 * - View OAuth2 callback success/error messages
 *
 * @example
 * ```tsx
 * <Route path="/sessions" element={<ThirdPartySessionsPage />} />
 * ```
 */
export const ThirdPartySessionsPage: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { sessions, loading, error, refetch } = useSessions();
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(
    null,
  );
  const [selectedSessionDetails, setSelectedSessionDetails] =
    useState<SessionDetail | null>(null);
  const [terminatingLoading, setTerminatingLoading] = useState(false);
  const [terminationError, setTerminationError] = useState<string | null>(null);
  const [alert, setAlert] = useState<AlertState | null>(null);
  const [refreshingServiceIds, setRefreshingServiceIds] = useState<Set<string>>(
    new Set(),
  );

  /**
   * Map OAuth2 error codes to user-friendly messages.
   * Handles standard OAuth2 error codes and custom error codes from the backend.
   */
  const mapErrorToMessage = (
    errorCode: string,
    description?: string | null,
  ): string => {
    const errorMap: Record<string, string> = {
      access_denied: 'You denied access to the service. No tokens were stored.',
      invalid_scope:
        'The requested permissions are not available. Please contact support.',
      expired_token: 'Your session expired. Please try again.',
      callback_failed: description || 'Authorization failed. Please try again.',
      invalid_callback: 'Invalid response from service. Please try again.',
      invalid_state: 'Invalid request state. Please try again.',
      invalid_redirect_uri:
        'Invalid redirect configuration. Please contact support.',
    };

    return errorMap[errorCode] || 'Authorization failed. Please try again.';
  };

  /**
   * Handle OAuth2 callback query parameters on component mount.
   * Parses success/error states and displays appropriate alerts.
   */
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const success = params.get('success');
    const errorCode = params.get('error');
    const errorDesc = params.get('error_description');

    if (success === 'true') {
      // Success state: Display success alert
      const message =
        'Successfully connected to service. You can now delegate access to agents.';
      setAlert({ type: 'success', message });

      // Refresh session list to show new session
      refetch();

      // Auto-dismiss after 5 seconds and clear URL
      const timer = setTimeout(() => {
        setAlert(null);
        navigate('/sessions', { replace: true });
      }, 5000);

      return () => clearTimeout(timer);
    } else if (errorCode) {
      // Error state: Display error alert
      const message = mapErrorToMessage(errorCode, errorDesc);
      setAlert({ type: 'error', message });

      // Clear URL immediately (don't auto-dismiss error alerts)
      navigate('/sessions', { replace: true });
    }
  }, [location.search, navigate, refetch]);

  /**
   * Handle alert dismissal.
   * Clears the alert state when user clicks dismiss button.
   */
  const handleAlertDismiss = () => {
    setAlert(null);
  };

  const handleTerminate = async (serviceId: string) => {
    try {
      setAlert(null);
      setTerminatingLoading(true);
      setTerminationError(null);

      // Fetch session details (including dependent agents)
      const details = await sessionsApi.getSessionDetails(serviceId);
      setSelectedSessionId(serviceId);
      setSelectedSessionDetails(details);
    } catch (err) {
      console.error('Failed to fetch session details', err);
      setAlert({
        type: 'error',
        message: 'Failed to load session details. Please try again.',
      });
    } finally {
      setTerminatingLoading(false);
    }
  };

  const handleTerminationConfirm = async () => {
    if (!selectedSessionId) return;

    try {
      setTerminatingLoading(true);
      setTerminationError(null);

      // Call API to terminate session (DELETE endpoint)
      await sessionsApi.terminateSession(selectedSessionId);

      // Close dialog and refetch sessions
      setSelectedSessionId(null);
      setSelectedSessionDetails(null);
      await refetch();

      // Show success message
      setAlert({
        type: 'success',
        message: 'Session terminated successfully.',
      });
    } catch (err) {
      console.error('Failed to terminate session', err);
      setTerminationError(
        extractApiError(err, 'Failed to terminate session. Please try again.'),
      );
    } finally {
      setTerminatingLoading(false);
    }
  };

  const handleTerminationCancel = () => {
    setSelectedSessionId(null);
    setSelectedSessionDetails(null);
    setTerminationError(null);
  };

  const handleRefresh = async (serviceId: string) => {
    try {
      setRefreshingServiceIds((prev) => new Set(prev).add(serviceId));
      setAlert(null);
      await sessionsApi.refreshSession(serviceId);
      await refetch();
      setAlert({
        type: 'success',
        message: 'Session token refreshed successfully.',
      });
    } catch (err) {
      console.error('Failed to refresh session token', err);
      setAlert({
        type: 'error',
        message: extractApiError(
          err,
          'Failed to refresh session token. Please try again.',
        ),
      });
    } finally {
      setRefreshingServiceIds((prev) => {
        const next = new Set(prev);
        next.delete(serviceId);
        return next;
      });
    }
  };

  // Loading state with skeleton cards
  if (loading) {
    return (
      <AppLayout>
        <PageTransition>
          <div className="space-y-6">
            {/* OAuth2 callback success/error alert */}
            {alert && (
              <Alert
                variant={alert.type}
                title={alert.type === 'success' ? 'Success' : 'Error'}
                dismissible
                onDismiss={handleAlertDismiss}
              >
                <p>{alert.message}</p>
              </Alert>
            )}

            {/* Page header */}
            <div>
              <h2 className="text-2xl font-semibold text-neutral-900">
                Third-Party Sessions
              </h2>
              <p className="mt-2 text-neutral-600">
                Manage your OAuth2 sessions with third-party services.
              </p>
            </div>

            {/* Loading skeleton cards */}
            <div>
              <h3 className="text-lg font-medium text-neutral-900 mb-4">
                Your Sessions
              </h3>
              <Grid
                columns={1}
                gap="md"
                className="sm:grid-cols-2 lg:grid-cols-3"
              >
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} variant="rounded" height="320px" />
                ))}
              </Grid>
            </div>
          </div>
        </PageTransition>
      </AppLayout>
    );
  }

  // Error state with retry button
  if (error) {
    return (
      <AppLayout>
        <PageTransition>
          <div className="space-y-6">
            {/* OAuth2 callback success/error alert */}
            {alert && (
              <Alert
                variant={alert.type}
                title={alert.type === 'success' ? 'Success' : 'Error'}
                dismissible
                onDismiss={handleAlertDismiss}
              >
                <p>{alert.message}</p>
              </Alert>
            )}

            {/* Page header */}
            <div>
              <h2 className="text-2xl font-semibold text-neutral-900">
                Third-Party Sessions
              </h2>
              <p className="mt-2 text-neutral-600">
                Manage your OAuth2 sessions with third-party services.
              </p>
            </div>

            {/* Error alert */}
            <Alert variant="error" title="Failed to Load Sessions">
              <p className="mb-4">{error}</p>
              <Button variant="outline" size="sm" onClick={refetch}>
                Retry
              </Button>
            </Alert>
          </div>
        </PageTransition>
      </AppLayout>
    );
  }

  // Empty state when no sessions exist
  if (sessions.length === 0) {
    return (
      <AppLayout>
        <PageTransition>
          <div className="space-y-6">
            {/* OAuth2 callback success/error alert */}
            {alert && (
              <Alert
                variant={alert.type}
                title={alert.type === 'success' ? 'Success' : 'Error'}
                dismissible
                onDismiss={handleAlertDismiss}
              >
                <p>{alert.message}</p>
              </Alert>
            )}

            {/* Page header */}
            <div>
              <h2 className="text-2xl font-semibold text-neutral-900">
                Third-Party Sessions
              </h2>
              <p className="mt-2 text-neutral-600">
                Manage your OAuth2 sessions with third-party services.
              </p>
            </div>

            {/* Subsection with empty state */}
            <div>
              <h3 className="text-lg font-medium text-neutral-900 mb-4">
                Your Sessions
              </h3>
              <EmptyState
                title="No Sessions Found"
                description="You haven't authenticated with any third-party services yet. Sessions will appear here once you grant permissions to agents."
              >
                <Button variant="primary" onClick={refetch}>
                  Refresh
                </Button>
              </EmptyState>
            </div>
          </div>
        </PageTransition>
      </AppLayout>
    );
  }

  // Main content with session cards
  return (
    <AppLayout>
      <PageTransition>
        <div className="space-y-6">
          {/* OAuth2 callback success/error alert */}
          {alert && (
            <Alert
              variant={alert.type}
              title={alert.type === 'success' ? 'Success' : 'Error'}
              dismissible
              onDismiss={handleAlertDismiss}
            >
              <p>{alert.message}</p>
            </Alert>
          )}

          {/* Page header */}
          <div>
            <h2 className="text-2xl font-semibold text-neutral-900">
              Third-Party Sessions
            </h2>
            <p className="mt-2 text-neutral-600">
              Manage your OAuth2 sessions with third-party services.
            </p>
          </div>

          {/* Subsection with session cards */}
          <div>
            <h3 className="text-lg font-medium text-neutral-900 mb-4">
              Your Sessions
            </h3>
            <Grid
              columns={1}
              gap="md"
              className="sm:grid-cols-2 lg:grid-cols-3"
            >
              {sessions.map((session) => (
                <SessionCard
                  key={session.id}
                  session={session}
                  onTerminate={handleTerminate}
                  onRefresh={handleRefresh}
                  refreshing={refreshingServiceIds.has(session.service_id)}
                  loading={
                    terminatingLoading &&
                    selectedSessionId === session.service_id
                  }
                />
              ))}
            </Grid>
          </div>
        </div>
      </PageTransition>

      {/* Termination confirmation dialog */}
      {selectedSessionId && selectedSessionDetails && (
        <TerminationDialog
          isOpen={!!selectedSessionId}
          onClose={handleTerminationCancel}
          onConfirm={handleTerminationConfirm}
          serviceName={selectedSessionDetails.session.service_display_name}
          dependentAgents={selectedSessionDetails.dependent_agents || []}
          loading={terminatingLoading}
          error={terminationError}
        />
      )}
    </AppLayout>
  );
};

export default ThirdPartySessionsPage;
