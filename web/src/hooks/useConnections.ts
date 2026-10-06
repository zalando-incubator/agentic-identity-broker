import { useEffect, useMemo, useState } from 'react';
import { useMutation, useMutationState, useQuery, useQueryClient } from '@tanstack/react-query';
import { isApiError } from '@services/api/client';
import { sessionsApi, type SessionSummary } from '@services/api/sessions';
import { usePrincipal } from '@services/query/QueryProvider';
import { queryKeys } from '@services/query/queryKeys';
import { deriveConnectionState, transitionConnectionState, type ConnectionEvent, type ConnectionState } from '@components/sessions/connectionState';
import { useOptimisticRevoke } from './optimisticRevoke';

export interface DisconnectConnectionOptions {
  onSuccess?: (serviceId: string) => void;
  onError?: (error: unknown, serviceId: string) => void;
}

export interface ConnectionsOptions {
  onRefreshSuccess?: (serviceId: string) => void;
  onRefreshError?: (serviceId: string) => void;
}

interface RefreshEvidence {
  event: Extract<ConnectionEvent, { type: 'refresh-failed' }>;
  readAt: number;
}
const noSessions: SessionSummary[] = [];
// Browsers overflow larger delays into near-immediate callbacks.
const MAX_TIMEOUT_MS = 2_147_483_647;

/** One shared list; refresh evidence is transient and never changes stored token facts. */
export function useConnections(options: ConnectionsOptions = {}) {
  const { principal } = usePrincipal();
  const client = useQueryClient();
  const listKey = queryKeys.connections(principal);
  const mutationKey = [...listKey, 'refresh'];
  const query = useQuery({
    queryKey: listKey,
    queryFn: async ({ signal }) => {
      const sessions = await sessionsApi.listSessions({ signal });
      return sessions.sort((left, right) =>
        Date.parse(right.initiated_at) - Date.parse(left.initiated_at) ||
        left.service_display_name.localeCompare(right.service_display_name) || right.service_id.localeCompare(left.service_id));
    },
  });
  const sessions = query.data ?? noSessions;
  const byService = useMemo(() => new Map(sessions.map(session => [session.service_id, session])), [sessions]);
  const [clock, setClock] = useState(Date.now);
  useEffect(() => {
    let nextDeadline = Infinity;
    for (const session of sessions) {
      if (session.is_expired || !session.has_refresh_token || !session.refresh_token_expires_at) continue;
      const deadline = Date.parse(session.refresh_token_expires_at);
      // Use the last update, not effect time, so a deadline crossed before this effect still rerenders.
      if (deadline > clock && deadline < nextDeadline) nextDeadline = deadline;
    }
    if (!Number.isFinite(nextDeadline)) return;
    const timer = window.setTimeout(() => setClock(Date.now()), Math.min(MAX_TIMEOUT_MS, Math.max(0, nextDeadline - Date.now())));
    return () => window.clearTimeout(timer);
  }, [sessions, clock]);
  const [evidence, setEvidence] = useState<Record<string, RefreshEvidence>>({});
  const [refreshError, setRefreshError] = useState<unknown>(null);
  useMutationState({ filters: { mutationKey, status: 'pending' }, select: mutation => mutation.state.variables });

  function getState(serviceId: string): ConnectionState {
    let state = deriveConnectionState({ context: 'sessions', session: byService.get(serviceId) });
    const prior = evidence[serviceId];
    if (prior && (prior.event.status === 409 || prior.event.status === 502 || prior.readAt === query.dataUpdatedAt)) {
      state = transitionConnectionState(state, prior.event);
    }
    return query.isError ? transitionConnectionState(state, { type: 'read-failed' }) : state;
  }

  const ownsRequest = (request: { serviceId: string }) => client.getMutationCache().find({
    mutationKey, status: 'pending', predicate: mutation => mutation.state.variables === request,
  }) !== undefined;
  const refreshMutation = useMutation({
    mutationKey,
    mutationFn: ({ serviceId }: { serviceId: string }) => sessionsApi.refreshSession(serviceId),
    retry: false,
    networkMode: 'always',
    gcTime: 0,
    onMutate: () => client.cancelQueries({ queryKey: listKey, exact: true }),
    onSuccess: (session, request) => {
      if (!ownsRequest(request)) return;
      setEvidence(previous => {
        const next = { ...previous };
        delete next[request.serviceId];
        return next;
      });
      client.setQueryData<SessionSummary[]>(listKey, records => records?.map(record =>
        record.service_id === request.serviceId ? session : record,
      ));
      void client.invalidateQueries({ queryKey: [...listKey, request.serviceId] });
      options.onRefreshSuccess?.(request.serviceId);
    },
    onError: (error, request) => {
      if (!ownsRequest(request)) return;
      const status = isApiError(error) ? error.status : undefined;
      setRefreshError(error);
      options.onRefreshError?.(request.serviceId);
      setEvidence(previous => ({ ...previous, [request.serviceId]: {
        event: { type: 'refresh-failed', status }, readAt: client.getQueryState(listKey)?.dataUpdatedAt ?? 0,
      } }));
      if (status === 404) return client.invalidateQueries({ queryKey: listKey, exact: true });
    },
  });

  const isRefreshing = (serviceId: string) => client.isMutating({
    mutationKey, predicate: mutation => (mutation.state.variables as { serviceId?: string } | undefined)?.serviceId === serviceId,
  }) > 0;

  async function refresh(serviceId: string) {
    if (isRefreshing(serviceId)) return;
    if (getState(serviceId).action !== 'refresh') {
      // A suspended tab can leave the old action visible until its deadline callback runs.
      setClock(Date.now());
      return;
    }
    setRefreshError(null);
    await refreshMutation.mutateAsync({ serviceId }).catch(() => undefined);
  }

  return { sessions, loading: query.isPending, error: query.error, refetch: query.refetch, getState, refresh, isRefreshing, refreshError };
}

export function useDisconnectConnection(options: DisconnectConnectionOptions = {}) {
  const { principal } = usePrincipal();
  return useOptimisticRevoke<SessionSummary>({
    listKey: queryKeys.connections(principal),
    recordId: session => session.service_id,
    mutationFn: serviceId => sessionsApi.terminateSession(serviceId),
    ...options,
  });
}
