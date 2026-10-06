import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import { DisconnectDialog } from '@components/sessions/DisconnectDialog';
import { useCollectionView } from '@hooks/useCollectionView';
import { useConnections, useDisconnectConnection } from '@hooks/useConnections';
import { connectionCallbackError, connectionsCopy } from '@copy/connections';
import type { SessionSummary } from '@services/api/sessions';
import { ConnectionsView, type ConnectionFilter, type ConnectionSort } from './ConnectionsView';

export function ConnectionsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [updated, setUpdated] = useState<{ serviceId: string; animationKey: number }>();
  const [focusAfterDisconnect, setFocusAfterDisconnect] = useState<string | null>(null);
  const callbackRef = useRef<string | null>(null);
  const returnFocusRef = useRef<string | null>(null);
  const headingRef = useRef<HTMLDivElement>(null);
  const connections = useConnections({
    onRefreshSuccess: serviceId => {
      setUpdated(previous => ({ serviceId, animationKey: (previous?.animationKey ?? 0) + 1 }));
      toast.success(connectionsCopy.refreshSuccess);
    },
    onRefreshError: () => toast.error(connectionsCopy.refreshError),
  });
  const disconnect = useDisconnectConnection({
    onSuccess: serviceId => {
      toast.success(connectionsCopy.disconnectSuccess);
      setFocusAfterDisconnect(serviceId);
    },
    onError: () => toast.error(connectionsCopy.disconnectError),
  });
  const { refetch } = connections;
  useEffect(() => {
    if (!focusAfterDisconnect || connections.sessions.some(session => session.service_id === focusAfterDisconnect)) return;
    if (document.activeElement === document.body || document.activeElement?.id === returnFocusRef.current) headingRef.current?.focus();
    setFocusAfterDisconnect(null);
  }, [connections.sessions, focusAfterDisconnect]);

  useEffect(() => {
    const callback = new URLSearchParams(location.search);
    const success = callback.get('success') === 'true';
    const error = callback.get('error');
    if (!success && !error) return;
    const callbackId = `${location.key}:${location.search}`;
    if (callbackRef.current === callbackId) return;
    callbackRef.current = callbackId;
    if (success) {
      const serviceId = callback.get('service_id');
      if (serviceId) setUpdated(previous => ({ serviceId, animationKey: (previous?.animationKey ?? 0) + 1 }));
      toast.success(connectionsCopy.callbackSuccess);
      void refetch();
    } else if (error) {
      toast.error(connectionCallbackError(error));
    }
    // Replace the result URL so reload and Back cannot replay the callback.
    navigate('/connections', { replace: true });
  }, [location.key, location.search, navigate, refetch]);

  const search = params.get('q') ?? '';
  const sortParam = params.get('sort');
  const sort: ConnectionSort = sortParam === 'name' || sortParam === 'recent' ? sortParam : 'attention';
  const filterParam = params.get('state');
  const filter: ConnectionFilter = filterParam === 'connected' || filterParam === 'needs-reauthentication' || filterParam === 'expired' || filterParam === 'error' ? filterParam : '';
  const { view, setView } = useCollectionView('connections', connections.sessions.length);

  const updateParam = (key: 'q' | 'sort' | 'state', value: string) => setParams(current => {
    const next = new URLSearchParams(current);
    if (value && !(key === 'sort' && value === 'attention')) next.set(key, value);
    else next.delete(key);
    return next;
  }, { replace: true });

  return <ConnectionsView sessions={connections.sessions} loading={connections.loading} error={Boolean(connections.error)}
    onRetry={() => { void connections.refetch(); }} getState={connections.getState} isRefreshing={connections.isRefreshing}
    isDisconnecting={disconnect.isPending} onRefresh={serviceId => { void connections.refresh(serviceId); }}
    onDisconnect={(session: SessionSummary) => {
      returnFocusRef.current = `disconnect-session-${session.id}`;
      disconnect.requestRevoke(session);
    }}
    search={search} onSearchChange={value => updateParam('q', value)} sort={sort} onSortChange={value => updateParam('sort', value)}
    filter={filter} onFilterChange={value => updateParam('state', value)} view={view} onViewChange={setView}
    updated={updated} headingRef={headingRef}
    dialog={<DisconnectDialog session={disconnect.confirmation} onCancel={disconnect.cancelRevoke} onConfirm={() => void disconnect.confirmRevoke()}
      onReturnFocus={() => {
        const target = returnFocusRef.current ? document.getElementById(returnFocusRef.current) : null;
        if (target && !target.matches(':disabled')) target.focus();
        else headingRef.current?.focus();
      }} />} />;
}
