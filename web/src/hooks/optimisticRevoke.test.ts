import { createElement, type ReactNode } from 'react';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import type { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { useOptimisticRevoke } from './optimisticRevoke';

type Record = { id: string; name: string };
const alice = { id: 'a', name: 'Agent A' };
const bob = { id: 'b', name: 'Agent B' };
const listKey = queryKeys.delegations('alice');
let client: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
beforeEach(() => { client = createQueryClient(); vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' }); client.setQueryData(listKey, [alice, bob]); });
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks(); });

function renderRevoke(mutationFn: (id: string) => Promise<unknown>) {
  return renderHook(() => useOptimisticRevoke<Record>({ listKey, detailKeys: (id) => [queryKeys.agent('alice', id), queryKeys.grant('alice', id)], recordId: (record) => record.id, mutationFn }), { wrapper });
}

describe('confirmed record-scoped revocation', () => {
  it('does nothing before confirmation, cancels reads, keeps a pending row until server success, and blocks duplicate actions', async () => {
    const response = Promise.withResolvers<void>();
    const revoke = vi.fn().mockReturnValue(response.promise);
    const { result } = renderRevoke(revoke);
    await waitFor(() => expect(result.current).not.toBeNull());
    act(() => { result.current.requestRevoke(alice); });
    expect(result.current.confirmation).toEqual(alice);
    expect(revoke).not.toHaveBeenCalled();
    act(() => { result.current.cancelRevoke(); });
    await act(async () => { await result.current.confirmRevoke(); });
    expect(revoke).not.toHaveBeenCalled();
    let signal: AbortSignal | undefined;
    void client.fetchQuery({ queryKey: listKey, queryFn: ({ signal: next }) => { signal = next; return Promise.withResolvers<Record[]>().promise; } }).catch(() => undefined);
    act(() => { result.current.requestRevoke(alice); });
    let completion!: Promise<void>;
    act(() => { completion = result.current.confirmRevoke(); });
    await waitFor(() => expect(revoke).toHaveBeenCalledTimes(1));
    expect(signal?.aborted).toBe(true);
    expect(result.current.confirmation).toBeNull();
    expect(result.current.isPending('a')).toBe(true);
    expect(result.current.isPending('b')).toBe(false);
    expect(client.getQueryData(listKey)).toEqual([alice, bob]);
    act(() => { result.current.requestRevoke(alice); });
    await act(async () => { await result.current.confirmRevoke(); });
    expect(revoke).toHaveBeenCalledTimes(1);
    await act(async () => { response.resolve(); await completion; });
    expect(client.getQueryData(listKey)).toEqual([bob]);
    expect(result.current.isPending('a')).toBe(false);
  });

  it('rolls back only the failed row and never resurrects an independently revoked row', async () => {
    const failed = Promise.withResolvers<void>();
    const succeeded = Promise.withResolvers<void>();
    const revoke = vi.fn((id: string) => id === 'a' ? failed.promise : succeeded.promise);
    const { result } = renderRevoke(revoke);
    await waitFor(() => expect(result.current).not.toBeNull());
    act(() => { result.current.requestRevoke(alice); });
    let first!: Promise<void>;
    act(() => { first = result.current.confirmRevoke(); });
    await waitFor(() => expect(result.current.isPending('a')).toBe(true));
    act(() => { result.current.requestRevoke(bob); });
    let second!: Promise<void>;
    act(() => { second = result.current.confirmRevoke(); });
    await waitFor(() => expect(result.current.isPending('b')).toBe(true));
    const newer = { id: 'c', name: 'New agent' };
    act(() => { client.setQueryData(listKey, [bob, newer]); });
    expect(result.current.isPending('a')).toBe(true);
    await act(async () => { succeeded.resolve(); await second; });
    expect(client.getQueryData(listKey)).toEqual([newer]);
    await act(async () => { failed.reject({ status: 500, message: 'failed' }); await first; });
    expect(client.getQueryData(listKey)).toEqual([alice, newer]);
    expect(result.current.isPending('a')).toBe(false);
    expect(result.current.error).toEqual({ status: 500, message: 'failed' });
    expect(revoke).toHaveBeenCalledTimes(2);
  });

  it.each(['success', 'failure'] as const)('ignores a removed request’s late %s when the same principal and record have a new pending revoke', async (outcome) => {
    const departed = Promise.withResolvers<void>();
    const current = Promise.withResolvers<void>();
    const revoke = vi.fn().mockReturnValueOnce(departed.promise).mockReturnValueOnce(current.promise);
    const success = vi.fn();
    const failure = vi.fn();
    const { result } = renderHook(() => useOptimisticRevoke<Record>({
      listKey,
      recordId: (record) => record.id,
      mutationFn: revoke,
      onSuccess: success,
      onError: failure,
    }), { wrapper });
    await waitFor(() => expect(result.current).not.toBeNull());
    act(() => { result.current.requestRevoke(alice); });
    let oldCompletion!: Promise<void>;
    act(() => { oldCompletion = result.current.confirmRevoke(); });
    await waitFor(() => expect(revoke).toHaveBeenCalledTimes(1));

    const replacement = { id: 'a', name: 'Newly granted access' };
    act(() => {
      client.getMutationCache().clear();
      client.removeQueries({ queryKey: queryKeys.principal('alice') });
      client.setQueryData(listKey, [replacement, bob]);
      result.current.requestRevoke(replacement);
    });
    let newCompletion!: Promise<void>;
    act(() => { newCompletion = result.current.confirmRevoke(); });
    await waitFor(() => expect(revoke).toHaveBeenCalledTimes(2));
    const authoritativeRows = outcome === 'success' ? [replacement, bob] : [bob];
    act(() => { client.setQueryData(listKey, authoritativeRows); });

    await act(async () => {
      if (outcome === 'success') departed.resolve();
      else departed.reject({ status: 500, message: 'Departed request failed' });
      await oldCompletion;
    });
    expect(client.getQueryData(listKey)).toEqual(authoritativeRows);
    expect(client.getQueryState(listKey)?.isInvalidated).toBe(false);
    expect(success).not.toHaveBeenCalled();
    expect(failure).not.toHaveBeenCalled();
    expect(result.current.error).toBeNull();
    expect(result.current.isPending('a')).toBe(true);

    await act(async () => { current.resolve(); await newCompletion; });
    expect(client.getQueryData(listKey)).toEqual([bob]);
    expect(success).toHaveBeenCalledExactlyOnceWith('a');
  });

  it('invalidates affected details, list and pending count after server settlement', async () => {
    const response = Promise.withResolvers<void>();
    const success = vi.fn();
    const keys = [listKey, queryKeys.agent('alice', 'a'), queryKeys.grant('alice', 'a'), queryKeys.pending('alice')];
    keys.slice(1).forEach((key) => client.setQueryData(key, { fresh: true }));
    const { result } = renderHook(() => useOptimisticRevoke<Record>({ listKey, detailKeys: () => keys.slice(1, 3), recordId: (record) => record.id, mutationFn: () => response.promise, onSuccess: success }), { wrapper });
    await waitFor(() => expect(result.current).not.toBeNull());
    act(() => { result.current.requestRevoke(alice); });
    let completion!: Promise<void>;
    act(() => { completion = result.current.confirmRevoke(); });
    await waitFor(() => expect(result.current.isPending('a')).toBe(true));
    expect(success).not.toHaveBeenCalled();
    await act(async () => { response.resolve(); await completion; });
    expect(success).toHaveBeenCalledWith('a');
    keys.forEach((key) => expect(client.getQueryState(key)?.isInvalidated).toBe(true));
  });
});
