import { createElement, type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import type { AgentDelegation } from '../types/consent';
import { useRevokeGrant } from './useRevokeGrant';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), deleteGrant: vi.fn() } }));
const aliceRows: AgentDelegation[] = ['a', 'b', 'c'].map((id) => ({ agentId: id, displayName: `Agent ${id}`, activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z' }));
const clients: QueryClient[] = [];
function deferred() {
  let resolve!: () => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function setup() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  clients.push(client);
  client.setQueryData(queryKeys.delegations('alice'), aliceRows);
  const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
  return { client, wrapper };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
});
afterEach(() => { clients.splice(0).forEach((client) => client.clear()); });

describe('record-scoped grant revoke', () => {
  it('cancel preserves the grant and never sends a delete', async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => useRevokeGrant(), { wrapper });
    await waitFor(() => expect(result.current?.confirmation).toBeNull());
    act(() => result.current.requestRevoke(aliceRows[0]!));
    expect(result.current.confirmation).toEqual(aliceRows[0]);
    act(() => result.current.cancelRevoke());
    expect(result.current.confirmation).toBeNull();
    expect(client.getQueryData(queryKeys.delegations('alice'))).toEqual(aliceRows);
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
  });

  it('isolates simultaneous records so a failed revoke cannot restore another successful revoke', async () => {
    const { client, wrapper } = setup();
    const a = deferred();
    const b = deferred();
    vi.mocked(consentApi.deleteGrant).mockImplementation((id) => id === 'a' ? a.promise : b.promise);
    const { result } = renderHook(() => useRevokeGrant(), { wrapper });
    await waitFor(() => expect(result.current?.confirmation).toBeNull());
    client.setQueryData(queryKeys.delegations('bob'), aliceRows);
    act(() => result.current.requestRevoke(aliceRows[0]!));
    let first!: Promise<void>;
    act(() => { first = result.current.confirmRevoke(); });
    await waitFor(() => expect(result.current.isPending('a')).toBe(true));
    expect(result.current.confirmation).toBeNull();
    expect(client.getQueryData(queryKeys.delegations('alice'))).toEqual(aliceRows);
    act(() => result.current.requestRevoke(aliceRows[1]!));
    let second!: Promise<void>;
    act(() => { second = result.current.confirmRevoke(); });
    await waitFor(() => expect(result.current.isPending('b')).toBe(true));
    act(() => client.setQueryData(queryKeys.delegations('alice'), [...aliceRows]));
    expect(result.current.isPending('a')).toBe(true);
    await act(async () => { b.resolve(); await second; });
    expect(client.getQueryData<AgentDelegation[]>(queryKeys.delegations('alice'))?.map((row) => row.agentId)).toEqual(['a', 'c']);
    await act(async () => { a.reject(new Error('Unavailable')); await first; });
    expect(client.getQueryData<AgentDelegation[]>(queryKeys.delegations('alice'))?.map((row) => row.agentId)).toEqual(['a', 'c']);
    expect(client.getQueryData(queryKeys.delegations('bob'))).toEqual(aliceRows);
    expect(result.current.isPending('a')).toBe(false);
    expect(consentApi.deleteGrant).toHaveBeenCalledTimes(2);
  });
});
