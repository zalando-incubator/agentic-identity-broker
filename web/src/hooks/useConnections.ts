import { useMemo, useState } from 'react';
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
}

interface RefreshEvidence {
  event: Extract<ConnectionEvent, { type: 'refresh-failed' }>;
  readAt: number;
}
const noSessions: SessionSummary[] = [];

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
    if (isRefreshing(serviceId) || getState(serviceId).action !== 'refresh') return;
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
