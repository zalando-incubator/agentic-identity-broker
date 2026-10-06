import { createElement, type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import type { AgentDelegation } from '../types/consent';
import { useDelegations } from './useDelegations';
import { useRevokeGrant } from './useRevokeGrant';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDelegations: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), deleteGrant: vi.fn() } }));
const rows: AgentDelegation[] = ['a', 'b'].map((id) => ({ agentId: id, displayName: `Agent ${id}`, activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z' }));
const clients: QueryClient[] = [];
function setup() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } });
  clients.push(client);
  const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
  return { client, wrapper };
}
function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDelegations).mockResolvedValue(rows);
});
afterEach(() => { vi.useRealTimers(); clients.splice(0).forEach((client) => client.clear()); });

describe('acting-user delegations', () => {
  it('never exposes another principal cache or fetches per-agent counts', async () => {
    const { client, wrapper } = setup();
    client.setQueryData(queryKeys.delegations('bob'), [{ ...rows[0], displayName: 'Private Bob agent' }]);
    const { result } = renderHook(() => useDelegations(), { wrapper });
    await waitFor(() => expect(result.current?.delegations).toEqual(rows));
    expect(client.getQueryData(queryKeys.delegations('alice'))).toEqual(rows);
    expect(consentApi.getAgentDelegations).toHaveBeenCalledWith({ signal: expect.any(AbortSignal) });
    expect(consentApi.getAgentDetail).not.toHaveBeenCalled();
    expect(consentApi.getAgentGrants).not.toHaveBeenCalled();
  });

  it('removes a delegation exactly at its deadline without a network refresh', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    const deadline = Date.now() + 60_000;
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([{ ...rows[0]!, expiresAt: new Date(deadline).toISOString() }, rows[1]!]);
    const { wrapper } = setup();
    const { result } = renderHook(() => useDelegations(), { wrapper });
    // IdentityBoundary commits before the child query can start.
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current.delegations).toHaveLength(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(59_997); });
    expect(result.current.delegations.map((row) => row.agentId)).toEqual(['a', 'b']);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current.delegations.map((row) => row.agentId)).toEqual(['b']);
    expect(consentApi.getAgentDelegations).toHaveBeenCalledTimes(1);
  });

  it('keeps a far-future delegation without overflowing the browser timer', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    const { wrapper } = setup();
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([{ ...rows[0]!, expiresAt: '2099-01-01T00:00:00Z' }]);
    const { result } = renderHook(() => useDelegations(), { wrapper });
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current.delegations.map((row) => row.agentId)).toEqual(['a']);
    await act(async () => { await vi.advanceTimersByTimeAsync(2_147_483_647); });
    expect(result.current.delegations.map((row) => row.agentId)).toEqual(['a']);
    expect(consentApi.getAgentDelegations).toHaveBeenCalledTimes(1);
  });

  it('keeps pending rows and isolates concurrent revoke success from failure', async () => {
    const { wrapper } = setup();
    const a = deferred(); const b = deferred();
    let currentRows = [...rows];
    vi.mocked(consentApi.getAgentDelegations).mockImplementation(async () => currentRows);
    vi.mocked(consentApi.deleteGrant).mockImplementation((id) => id === 'a' ? a.promise : b.promise);
    const { result } = renderHook(() => ({ list: useDelegations(), revoke: useRevokeGrant() }), { wrapper });
    await waitFor(() => expect(result.current?.list.delegations).toEqual(rows));
    act(() => result.current.revoke.requestRevoke(rows[0]!));
    act(() => { void result.current.revoke.confirmRevoke(); });
    await waitFor(() => expect(result.current.revoke.isPending('a')).toBe(true));
    act(() => result.current.revoke.requestRevoke(rows[1]!));
    act(() => { void result.current.revoke.confirmRevoke(); });
    await waitFor(() => expect(result.current.revoke.isPending('b')).toBe(true));
    expect(result.current.list.delegations).toEqual(rows);
    currentRows = [rows[0]!];
    await act(async () => b.resolve());
    await waitFor(() => expect(result.current.list.delegations).toEqual([rows[0]]));
    await act(async () => a.reject(new Error('Unavailable')));
    await waitFor(() => expect(result.current.revoke.isPending('a')).toBe(false));
    expect(result.current.list.delegations).toEqual([rows[0]]);
  });
});
