import { createElement, type ReactNode } from 'react';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { approvalApi } from '@services/api/approvals';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import type { ToolApprovalDetail } from '../types/approval';
import { useStandingApprovals } from './useStandingApprovals';

const record: ToolApprovalDetail = { id: 'allow', principal: 'alice', agent_id: 'agent', tool_name: 'read', arguments: {}, tool_pattern: 'read', params_pattern: {}, pattern_preview: 'read()', status: 'approved', persistence: 'permanent', approval_url: '/approvals/allow', created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z' };
let client: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
beforeEach(() => {
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
});
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks(); });

it('does not reuse another principal’s standing decisions', async () => {
  vi.spyOn(approvalApi, 'listPermanentApprovals').mockResolvedValueOnce([record]).mockResolvedValue([]);
  const { result } = renderHook(() => useStandingApprovals(), { wrapper });
  await waitFor(() => expect(result.current?.data).toEqual([record]));
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
  await act(async () => { await client.invalidateQueries({ queryKey: ['identity'] }); });
  await waitFor(() => expect(result.current?.data).toEqual([]));
  expect(client.getQueryData(queryKeys.standing('alice'))).toBeUndefined();
});

it('requires confirmation, retains the row pending, and removes it only on server success', async () => {
  vi.spyOn(approvalApi, 'listPermanentApprovals').mockResolvedValue([record]);
  const response = Promise.withResolvers<{ id: string; status: string; denied_at: string }>();
  const revoke = vi.spyOn(approvalApi, 'revokePermanentApproval').mockReturnValue(response.promise);
  const { result } = renderHook(() => useStandingApprovals(), { wrapper });
  await waitFor(() => expect(result.current?.data).toEqual([record]));
  act(() => result.current.revoke.requestRevoke(record));
  expect(revoke).not.toHaveBeenCalled();
  act(() => { void result.current.revoke.confirmRevoke(); });
  await waitFor(() => expect(revoke).toHaveBeenCalledWith('allow'));
  expect(result.current.revoke.confirmation).toBeNull();
  expect(result.current.revoke.isPending('allow')).toBe(true);
  expect(result.current.data).toEqual([record]);
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([]);
  await act(async () => { response.resolve({ id: 'allow', status: 'denied', denied_at: '2026-09-01T00:00:00Z' }); });
  await waitFor(() => expect(result.current.data).toEqual([]));
});

it('keeps failed rows without restoring an unrelated successful revoke', async () => {
  const denied = { ...record, id: 'deny', status: 'denied' as const };
  let records = [record, denied];
  vi.spyOn(approvalApi, 'listPermanentApprovals').mockImplementation(async () => records);
  const response = Promise.withResolvers<{ id: string; status: string; denied_at: string }>();
  vi.spyOn(approvalApi, 'revokePermanentApproval').mockImplementation(id => id === 'allow'
    ? response.promise
    : Promise.resolve({ id, status: 'denied', denied_at: '2026-09-01T00:00:00Z' }));
  const { result } = renderHook(() => useStandingApprovals(), { wrapper });
  await waitFor(() => expect(result.current?.data).toEqual(records));
  act(() => result.current.revoke.requestRevoke(record));
  act(() => { void result.current.revoke.confirmRevoke(); });
  await waitFor(() => expect(result.current.revoke.isPending('allow')).toBe(true));
  records = [record];
  act(() => result.current.revoke.requestRevoke(denied));
  await act(async () => { await result.current.revoke.confirmRevoke(); });
  await act(async () => { response.reject(new Error('offline')); });
  await waitFor(() => expect(result.current.revoke.error).toBeTruthy());
  expect(result.current.data).toEqual([record]);
});

it('keeps standing rows stable across refreshes with tied creation times', async () => {
  const first = { ...record, id: 'first', created_at: '2025-12-31T19:00:00-05:00' };
  const second = { ...record, id: 'second' };
  const newest = { ...record, id: 'newest', created_at: '2026-01-02T00:00:00Z' };
  vi.spyOn(approvalApi, 'listPermanentApprovals')
    .mockResolvedValueOnce([newest, second, first])
    .mockResolvedValueOnce([second, first, newest]);
  const { result } = renderHook(() => useStandingApprovals(), { wrapper });
  await waitFor(() => expect(result.current.data?.map(item => item.id)).toEqual(['newest', 'first', 'second']));
  await act(async () => { await result.current.refetch(); });
  expect(result.current.data?.map(item => item.id)).toEqual(['newest', 'first', 'second']);
});
