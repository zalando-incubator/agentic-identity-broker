import { createElement, type ReactNode } from 'react';
import { act, cleanup, renderHook } from '@testing-library/react';
import type { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { approvalApi } from '@services/api/approvals';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import type { ToolApprovalDetail } from '../types/approval';
import { PendingApprovalsProvider, usePendingApprovals } from './usePendingApprovals';

const approval: ToolApprovalDetail = { id: 'one', principal: 'alice', agent_id: 'agent', tool_name: 'read', arguments: {}, tool_pattern: 'read', params_pattern: {}, pattern_preview: 'read()', status: 'pending', approval_url: '/approvals/one', created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z' };
let client: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, createElement(PendingApprovalsProvider, null, children));
async function flush() { await act(async () => { await vi.advanceTimersByTimeAsync(1); }); }

beforeEach(() => {
  vi.useFakeTimers();
  Object.defineProperty(document, 'hidden', { configurable: true, value: false });
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
});
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks(); vi.useRealTimers(); Object.defineProperty(document, 'hidden', { configurable: true, value: false }); });

describe('shared pending approvals', () => {
  it('refreshes visible consumers every ten seconds without polling while hidden', async () => {
    const read = vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValue([approval]);
    const standing = vi.spyOn(approvalApi, 'listPermanentApprovals');
    const { result } = renderHook(() => ({ queue: usePendingApprovals(), sidebar: usePendingApprovals() }), { wrapper });
    await flush(); await flush();
    expect(result.current).not.toBeNull();
    expect(result.current.queue.count).toBe(1);
    expect(result.current.sidebar.count).toBe(1);
    expect(read).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(read).toHaveBeenCalledTimes(2);
    Object.defineProperty(document, 'hidden', { configurable: true, value: true });
    act(() => document.dispatchEvent(new Event('visibilitychange')));
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(read).toHaveBeenCalledTimes(2);
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
    act(() => window.dispatchEvent(new Event('focus')));
    await flush();
    expect(read).toHaveBeenCalledTimes(3);
    expect(standing).not.toHaveBeenCalled();
  });

  it.each(['approve', 'deny', 'revoke'])('refreshes immediately after a %s settlement', async (action) => {
    vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValueOnce([approval]).mockResolvedValue([]);
    const { result } = renderHook(() => usePendingApprovals(), { wrapper });
    await flush(); await flush();
    expect(result.current).not.toBeNull();
    expect(result.current.count).toBe(1);
    await act(async () => { await client.getMutationCache().build(client, { mutationKey: [...queryKeys.principal('alice'), action], mutationFn: async () => undefined }).execute(undefined); });
    await flush();
    expect(result.current.count).toBe(0);
  });

  it('preserves the last known list and marks a failed refresh stale', async () => {
    vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValueOnce([approval]).mockRejectedValue({ status: 0, message: 'offline' });
    const { result } = renderHook(() => usePendingApprovals(), { wrapper });
    await flush(); await flush();
    expect(result.current).not.toBeNull();
    expect(result.current.count).toBe(1);
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.pending('alice') }); });
    await flush();
    expect(result.current.count).toBe(1);
    expect(result.current.data).toEqual([approval]);
    expect(result.current.stale).toBe(true);
  });

  it('does not invent a zero count before any successful read', async () => {
    vi.spyOn(approvalApi, 'listPendingApprovals').mockRejectedValue({ status: 0, message: 'offline' });
    const { result } = renderHook(() => usePendingApprovals(), { wrapper });
    await flush(); await flush();
    expect(result.current).not.toBeNull();
    expect(result.current.count).toBeUndefined();
    expect(result.current.stale).toBe(true);
  });
});
