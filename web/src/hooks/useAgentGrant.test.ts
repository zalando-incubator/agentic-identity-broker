import { createElement, type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { isCancelledError, type QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi, type AgentDetailData } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import type { AgentDelegation, GrantResult, UserGrant } from '../types/consent';
import { useAgentGrant, useSaveGrant } from './useAgentGrant';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn() } }));
const grant: UserGrant = { id: 'grant-a', agent_id: 'agent-a', principal: 'alice', granted_permission_sets: { read: ['mail'] }, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const clients: QueryClient[] = [];
function setup() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  clients.push(client);
  return { client, wrapper: ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children) };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue(grant);
});
afterEach(() => { clients.splice(0).forEach((client) => client.clear()); });

describe('principal-scoped agent grants', () => {
  it('cannot expose another principal or another agent cached grant', async () => {
    const { client, wrapper } = setup();
    client.setQueryData(queryKeys.grant('bob', 'agent-a'), { ...grant, principal: 'bob' });
    client.setQueryData(queryKeys.grant('alice', 'agent-b'), { ...grant, agent_id: 'agent-b' });
    const { result } = renderHook(() => useAgentGrant('agent-a'), { wrapper });
    await waitFor(() => expect(result.current.data).toEqual(grant));
    expect(client.getQueryData(queryKeys.grant('alice', 'agent-a'))).toEqual(grant);
    expect(client.getQueryData(queryKeys.grant('bob', 'agent-a'))).toBeUndefined();
  });

  it('keeps server grant authoritative during save and invalidates its dependent views after success', async () => {
    const { client, wrapper } = setup();
    client.setQueryData(queryKeys.delegations('alice'), [{ agentId: 'agent-a', displayName: 'Agent A', activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z' } satisfies AgentDelegation]);
    client.setQueryData(queryKeys.agent('alice', 'agent-a'), {
      agent: { agentId: 'agent-a', displayName: 'Agent A', description: 'Agent A', permission_sets: [], active_session_service_ids: [], service_requirements: [] },
      services: [],
    } satisfies AgentDetailData);
    const response = deferred<GrantResult>();
    vi.mocked(consentApi.createOrUpdateGrant).mockReturnValue(response.promise);
    const { result } = renderHook(() => ({ read: useAgentGrant('agent-a'), save: useSaveGrant('agent-a', { sessionToken: 'authorization' }) }), { wrapper });
    await waitFor(() => expect(result.current.read.data).toEqual(grant));
    client.setQueryData(queryKeys.grant('bob', 'agent-a'), { ...grant, principal: 'bob' });
    const request = { granted_permission_sets: { read: ['mail'], calendar: ['calendar'] } };
    act(() => result.current.save.mutate(request));
    await waitFor(() => expect(result.current.save.isPending).toBe(true));
    expect(result.current.read.data).toEqual(grant);
    expect(client.getQueryState(queryKeys.delegations('alice'))?.isInvalidated).toBe(false);
    const updated = { ...grant, granted_permission_sets: request.granted_permission_sets };
    vi.mocked(consentApi.getAgentGrants).mockResolvedValue(updated);
    await act(async () => { response.resolve({ kind: 'created', grant: updated }); });
    await waitFor(() => expect(result.current.read.data).toEqual(updated));
    expect(client.getQueryState(queryKeys.delegations('alice'))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.agent('alice', 'agent-a'))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.grant('bob', 'agent-a'))?.isInvalidated).toBe(false);
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledWith('agent-a', request, { sessionToken: 'authorization' });
  });

  it('retains the previous grant and never retries a rejected security mutation', async () => {
    const { wrapper } = setup();
    vi.mocked(consentApi.createOrUpdateGrant).mockRejectedValue({ status: 403, code: 'DENIED', message: 'Not permitted' });
    const { result } = renderHook(() => ({ read: useAgentGrant('agent-a'), save: useSaveGrant('agent-a') }), { wrapper });
    await waitFor(() => expect(result.current.read.data).toEqual(grant));
    act(() => result.current.save.mutate({ granted_permission_sets: {} }));
    await waitFor(() => expect(result.current.save.isError).toBe(true));
    expect(result.current.read.data).toEqual(grant);
    expect(consentApi.createOrUpdateGrant).toHaveBeenCalledTimes(1);
  });
  it('rejects a departed continuation when identity changes during post-save invalidation', async () => {
    const { client, wrapper } = setup();
    const refresh = deferred<UserGrant | null>();
    vi.mocked(consentApi.getAgentGrants)
      .mockResolvedValueOnce(grant)
      .mockReturnValueOnce(refresh.promise)
      .mockResolvedValue({ ...grant, principal: 'bob' });
    vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'redirect', redirectUrl: '/authorized' });
    const { result } = renderHook(() => ({ read: useAgentGrant('agent-a'), save: useSaveGrant('agent-a') }), { wrapper });
    await waitFor(() => expect(result.current.read.data).toEqual(grant));
    let continuation: GrantResult | undefined;
    let failure: unknown;
    let completed!: Promise<void>;
    act(() => {
      completed = result.current.save.mutateAsync({ granted_permission_sets: grant.granted_permission_sets })
        .then((value) => { continuation = value; }, (error: unknown) => { failure = error; });
    });
    await waitFor(() => expect(consentApi.getAgentGrants).toHaveBeenCalledTimes(2));
    vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
    await act(async () => { await client.refetchQueries({ queryKey: ['identity'] }); });
    await act(async () => { refresh.resolve(grant); await completed; });
    expect(continuation).toBeUndefined();
    expect(isCancelledError(failure)).toBe(true);
    await waitFor(() => {
      expect(result.current.read.data?.principal).toBe('bob');
      expect(client.getQueryData(queryKeys.grant('alice', 'agent-a'))).toBeUndefined();
    });
  });
});
