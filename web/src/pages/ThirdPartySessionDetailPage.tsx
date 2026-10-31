/**
 * ThirdPartySessionDetailPage — `/sessions/:service`
 *
 * The page behind a Virtual Service Scope (specs/055-incremental-service-authorization): one
 * service's identity, the signed-in user's session state, the user's dependent agents, and
 * Login, Refresh and Terminate. Signing in here repairs the session only; it never grants an
 * agent permission.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { AppLayout } from '@components/layout/AppLayout';
import { PageTransition } from '@components/ui/PageTransition';
import { TerminationDialog } from '@components/sessions/TerminationDialog';
import { Card } from '@design-system/components/data-display/Card';
import { Alert } from '@design-system/components/feedback/Alert';
import { EmptyState } from '@design-system/components/feedback/EmptyState';
import { Skeleton } from '@design-system/components/feedback/Skeleton';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import { Stack } from '@design-system/components/layout/Stack';
import { sessionsApi, type ServiceSessionDetail } from '@services/api/sessions';
import { isApiError } from '@services/api/client';
import { extractApiError } from '@utils/api';

type LoadState =
  | { kind: 'loading' }
  | { kind: 'not_found' }
  | { kind: 'error'; message: string }
  | { kind: 'loaded'; detail: ServiceSessionDetail };

interface AlertState {
  type: 'success' | 'error';
  message: string;
}

const formatTime = (value?: string) => (value ? new Date(value).toLocaleString() : 'Unknown');

export const ThirdPartySessionDetailPage: React.FC = () => {
  const { service: identifier = '' } = useParams<{ service: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const [state, setState] = useState<LoadState>({ kind: 'loading' });
  const [alert, setAlert] = useState<AlertState | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [confirmingTermination, setConfirmingTermination] = useState(false);
  const [terminating, setTerminating] = useState(false);
  const [terminationError, setTerminationError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const detail = await sessionsApi.getServiceSessionDetail(identifier);
      setState({ kind: 'loaded', detail });
      return detail;
    } catch (err) {
      if (isApiError(err) && err.status === 404) {
        setState({ kind: 'not_found' });
      } else {
        setState({ kind: 'error', message: extractApiError(err, 'Failed to load the service session.') });
      }
      return null;
    }
  }, [identifier]);

  useEffect(() => {
    setState({ kind: 'loading' });
    void load();
  }, [load]);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    if (params.get('success') === 'true') {
      setAlert({ type: 'success', message: 'Successfully connected to service.' });
      navigate(location.pathname, { replace: true });
    } else if (params.get('error')) {
      setAlert({ type: 'error', message: 'Authorization failed. Please try again.' });
      navigate(location.pathname, { replace: true });
    }
  }, [location.pathname, location.search, navigate]);

  const handleLogin = (serviceID: string) => {
    const returnURL = `${window.location.origin}/sessions/${encodeURIComponent(identifier)}`;
    window.location.assign(
      `/api/third-party/${serviceID}/oauth2/authorize?redirect_uri=${encodeURIComponent(returnURL)}`,
    );
  };

  const handleRefresh = async (serviceID: string) => {
    setRefreshing(true);
    setAlert(null);
    try {
      await sessionsApi.refreshSession(serviceID);
      await load();
      setAlert({ type: 'success', message: 'Session token refreshed successfully.' });
    } catch (err) {
      const detail = await load();
      setAlert({
        type: 'error',
        message: detail?.session?.is_expired
          ? 'This session can no longer be renewed. Log in to reconnect.'
          : extractApiError(err, 'Failed to refresh session token. Please try again.'),
      });
    } finally {
      setRefreshing(false);
    }
  };

  const handleTerminationConfirm = async (serviceID: string) => {
    setTerminating(true);
    setTerminationError(null);
    try {
      await sessionsApi.terminateSession(serviceID);
      setConfirmingTermination(false);
      await load();
      setAlert({ type: 'success', message: 'Session terminated successfully.' });
    } catch (err) {
      setTerminationError(extractApiError(err, 'Failed to terminate session. Please try again.'));
    } finally {
      setTerminating(false);
    }
  };

  const renderBody = () => {
    switch (state.kind) {
      case 'loading':
        return <Skeleton height="12rem" />;
      case 'not_found':
        return (
          <EmptyState
            title="Service not found"
            description="No third-party service matches this link. Check the address or open your sessions list."
            action={{ label: 'View all sessions', onClick: () => navigate('/sessions') }}
          />
        );
      case 'error':
        return (
          <Alert variant="error" title="Unable to load session">
            <p>{state.message}</p>
            <div className="mt-3">
              <Button variant="secondary" size="sm" onClick={() => void load()}>
                Retry
              </Button>
            </div>
          </Alert>
        );
      case 'loaded': {
        const { service, session, dependent_agents: agents } = state.detail;
        const needsLogin = !session || session.is_expired;
        return (
          <Stack spacing="lg">
            <div>
              <h2 className="font-display text-2xl font-semibold text-trust-deep">{service.display_name}</h2>
              <p className="mt-1 font-mono text-sm text-neutral-600 break-all">{service.scope_url}</p>
            </div>

            <Card>
              <div data-testid="service-session-status" className="space-y-3">
                {!session && (
                  <>
                    <Badge variant="neutral">No active session</Badge>
                    <p className="text-neutral-700">
                      Sign in to {service.display_name} so your agents can use it. Signing in does not
                      grant any agent new permissions.
                    </p>
                  </>
                )}
                {session && session.is_expired && (
                  <>
                    <Badge variant="error">Expired</Badge>
                    <p className="text-neutral-700">This session can no longer be renewed. Log in to reconnect.</p>
                  </>
                )}
                {session && !session.is_expired && (
                  <>
                    <Badge variant="success">Active</Badge>
                    <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
                      <div>
                        <dt className="text-neutral-600">Connected</dt>
                        <dd className="text-neutral-900">{formatTime(session.initiated_at)}</dd>
                      </div>
                      <div>
                        <dt className="text-neutral-600">Access token expires</dt>
                        <dd className="text-neutral-900">{formatTime(session.access_token_expires_at)}</dd>
                      </div>
                    </dl>
                  </>
                )}
                {session && session.scope.length > 0 && (
                  <ul aria-label="Granted scopes" className="flex flex-wrap gap-2">
                    {session.scope.map((scope) => (
                      <li key={scope}>
                        <Badge variant="info" className="font-mono">{scope}</Badge>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              <div className="mt-4 flex flex-wrap gap-3">
                {needsLogin ? (
                  <Button variant="primary" onClick={() => handleLogin(service.id)}>
                    Login
                  </Button>
                ) : (
                  <>
                    <Button variant="secondary" loading={refreshing} onClick={() => void handleRefresh(service.id)}>
                      Refresh
                    </Button>
                    <Button variant="danger" onClick={() => setConfirmingTermination(true)}>
                      Terminate
                    </Button>
                  </>
                )}
              </div>
            </Card>

            <section aria-labelledby="dependent-agents-heading">
              <h3 id="dependent-agents-heading" className="text-lg font-medium text-neutral-900">
                Agents using this service
              </h3>
              {agents.length === 0 ? (
                <p className="mt-2 text-neutral-600">None of your agents use this service.</p>
              ) : (
                <ul data-testid="service-dependent-agents" className="mt-2 space-y-1 text-neutral-800">
                  {agents.map((agent) => (
                    <li key={agent.agent_id}>{agent.display_name}</li>
                  ))}
                </ul>
              )}
            </section>

            {session && (
              <TerminationDialog
                isOpen={confirmingTermination}
                onClose={() => {
                  setConfirmingTermination(false);
                  setTerminationError(null);
                }}
                onConfirm={() => void handleTerminationConfirm(service.id)}
                serviceName={service.display_name}
                dependentAgents={agents.map((agent) => ({ id: agent.agent_id, display_name: agent.display_name }))}
                loading={terminating}
                error={terminationError}
              />
            )}
          </Stack>
        );
      }
    }
  };

  return (
    <AppLayout>
      <PageTransition>
        <div className="space-y-6">
          {alert && (
            <Alert
              variant={alert.type}
              title={alert.type === 'success' ? 'Success' : 'Error'}
              dismissible
              onDismiss={() => setAlert(null)}
            >
              <p>{alert.message}</p>
            </Alert>
          )}
          {renderBody()}
        </div>
      </PageTransition>
    </AppLayout>
  );
};

export default ThirdPartySessionDetailPage;
