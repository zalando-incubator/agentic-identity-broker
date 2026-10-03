import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { PageHeader } from '@design-system/components/layout/PageHeader';
import { Button } from '@design-system/components/primitives/Button';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { ConnectionsTable } from '@components/sessions/ConnectionsTable';
import { DisconnectDialog } from '@components/sessions/DisconnectDialog';
import { useConnections, useDisconnectConnection } from '@hooks/useConnections';
import { commonCopy, navigationCopy } from '@copy';
import { connectionCallbackError, connectionsCopy } from '@copy/connections';
import { extractApiError } from '@utils/api';

interface ConnectionNotice {
  type: 'success' | 'error';
  message: string;
}

export function ConnectionsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [notice, setNotice] = useState<ConnectionNotice | null>(null);
  const callbackRef = useRef<string | null>(null);
  const returnFocusRef = useRef<string | null>(null);
  const pageRef = useRef<HTMLDivElement>(null);
  const connections = useConnections({ onRefreshSuccess: () => setNotice({ type: 'success', message: connectionsCopy.refreshSuccess }) });
  const disconnect = useDisconnectConnection({
    onSuccess: () => setNotice({ type: 'success', message: connectionsCopy.disconnectSuccess }),
    onError: error => setNotice({ type: 'error', message: extractApiError(error, connectionsCopy.disconnectError) }),
  });
  const { refetch } = connections;

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const success = params.get('success') === 'true';
    const error = params.get('error');
    if (!success && !error) return;
    const callbackId = `${location.key}:${location.search}`;
    if (callbackRef.current === callbackId) return;
    callbackRef.current = callbackId;
    if (success) {
      setNotice({ type: 'success', message: connectionsCopy.callbackSuccess });
      void refetch();
    } else if (error) {
      setNotice({ type: 'error', message: connectionCallbackError(error, params.get('error_description')) });
    }
    // Consume callback parameters once. Refreshes and Back must not replay a callback.
    navigate('/sessions', { replace: true });
  }, [location.key, location.search, navigate, refetch]);

  useEffect(() => {
    if (notice?.type !== 'success') return;
    const timer = window.setTimeout(() => setNotice(null), 5000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  return <div ref={pageRef} tabIndex={-1} className="min-w-0 space-y-4 rounded-md outline-none focus:ring-2 focus:ring-inset focus:ring-ring">
    <PageHeader title={navigationCopy.connections} purpose={connectionsCopy.purpose} />
    {notice && <div role={notice.type === 'error' ? 'alert' : 'status'} className="flex items-start justify-between gap-3 rounded-md border border-border bg-muted p-3">
      <TruncatedText text={notice.message} as="p" lines={2} expandLabel={commonCopy.showMore} collapseLabel={commonCopy.showLess} />
      <Button variant="ghost" size="sm" onClick={() => setNotice(null)}>{commonCopy.close}</Button>
    </div>}
    {connections.refreshError != null && <p role="alert" className="text-sm text-destructive">{extractApiError(connections.refreshError, connectionsCopy.refreshError)}</p>}
    {connections.error && <div role="alert" className="space-y-2">
      <p className="text-sm">{connectionsCopy.readError}</p>
      <Button variant="outline" onClick={() => void connections.refetch()}>{commonCopy.retry}</Button>
    </div>}
    {connections.loading ? <p role="status">{commonCopy.loading}</p> : connections.sessions.length > 0 ? <ConnectionsTable
      sessions={connections.sessions}
      getState={connections.getState}
      isRefreshing={connections.isRefreshing}
      isDisconnecting={disconnect.isPending}
      onRefresh={serviceId => { setNotice(null); void connections.refresh(serviceId); }}
      onDisconnect={session => {
        returnFocusRef.current = `disconnect-session-${session.id}`;
        disconnect.requestRevoke(session);
      }}
      onRetry={() => void connections.refetch()}
    /> : !connections.error && <p className="text-sm text-muted-foreground">{connectionsCopy.empty}</p>}
    <DisconnectDialog session={disconnect.confirmation} onCancel={disconnect.cancelRevoke} onConfirm={() => void disconnect.confirmRevoke()} onReturnFocus={() => {
      const target = returnFocusRef.current ? document.getElementById(returnFocusRef.current) : null;
      if (target && !target.matches(':disabled')) target.focus();
      else pageRef.current?.focus();
    }} />
  </div>;
}
