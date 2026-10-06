import { createElement, type ReactNode } from 'react';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AxiosError } from 'axios';
import { approvalApi } from '@services/api/approvals';
import type { QueryClient } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { advanceAuthGeneration, apiClient, getAuthGeneration } from '@services/api/client';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import type { ToolApprovalDetail, ApproveResponseData, DenyResponseData } from '../types/approval';
import { useApprovalReview } from './useApprovalReview';

const approval: ToolApprovalDetail = { id: 'one', principal: 'alice', agent_id: 'agent', tool_name: 'read', arguments: {}, tool_pattern: 'read', params_pattern: {}, pattern_preview: 'read()', status: 'pending', approval_url: '/approvals/one', created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z' };
let client: QueryClient;
const adapter = apiClient.defaults.adapter;
const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children);
beforeEach(() => {
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(approvalApi, 'getApproval').mockResolvedValue(approval);
});
afterEach(() => { cleanup(); client.clear(); apiClient.defaults.adapter = adapter; vi.restoreAllMocks(); });

describe('approval review server ownership', () => {
  it('isolates cached details by the authenticated principal and route ID', async () => {
    client.setQueryData(queryKeys.approval('bob', 'one'), { ...approval, principal: 'bob', status: 'approved' });
    const { result, rerender } = renderHook(({ id }) => useApprovalReview(id), { initialProps: { id: 'one' }, wrapper });
    await waitFor(() => expect(result.current?.approval?.status).toBe('pending'));
    expect(client.getQueryData(queryKeys.approval('alice', 'one'))).toEqual(approval);
    vi.mocked(approvalApi.getApproval).mockResolvedValue({ ...approval, id: 'two', tool_name: 'write' });
    rerender({ id: 'two' });
    await waitFor(() => expect(result.current.approval?.tool_name).toBe('write'));
    expect(client.getQueryData(queryKeys.approval('alice', 'two'))).toMatchObject({ id: 'two' });
  });

  it.each(['approve', 'deny'] as const)('does not record %s before the server accepts it, and invalidates pending on settlement', async (action) => {
    const approved = Promise.withResolvers<ApproveResponseData>();
    const denied = Promise.withResolvers<DenyResponseData>();
    const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(approved.promise);
    const deny = vi.spyOn(approvalApi, 'denyApproval').mockReturnValue(denied.promise);
    const mutation = action === 'approve' ? approve : deny;
    client.setQueryData(queryKeys.pending('alice'), [approval]);
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current?.approval).toEqual(approval));
    act(() => { void (action === 'approve' ? result.current.approve({ persistence: 'once' }) : result.current.deny()); });
    await waitFor(() => expect(result.current.submitting).toBe(true));
    expect(result.current.submittingAction).toBe(action);
    await act(async () => { void (action === 'approve' ? result.current.deny() : result.current.approve({ persistence: 'once' })); });
    expect(action === 'approve' ? deny : approve).not.toHaveBeenCalled();
    expect(result.current.approval?.status).toBe('pending');
    expect(result.current.approveResult).toBeNull();
    expect(result.current.denyResult).toBeNull();
    expect(client.getQueryData(queryKeys.pending('alice'))).toEqual([approval]);
    await act(async () => {
      if (action === 'approve') approved.resolve({ id: 'one', status: 'approved', persistence: 'once', approved_at: '2026-01-01T00:01:00Z' });
      else denied.resolve({ id: 'one', status: 'denied', denied_at: '2026-01-01T00:01:00Z' });
    });
    await waitFor(() => expect(action === 'approve' ? result.current.approveResult : result.current.denyResult).not.toBeNull());
    expect(result.current.submittingAction).toBeNull();
    expect(mutation).toHaveBeenCalledTimes(1);
    expect(client.getQueryState(queryKeys.pending('alice'))?.isInvalidated).toBe(true);
  });

  it('keeps other decisions blocked until an in-flight decision finishes across request selection', async () => {
    const response = Promise.withResolvers<ApproveResponseData>();
    const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(response.promise);
    const deny = vi.spyOn(approvalApi, 'denyApproval').mockResolvedValue({ id: 'two', status: 'denied', denied_at: '2026-01-01T00:01:00Z' });
    vi.mocked(approvalApi.getApproval).mockImplementation(async id => ({ ...approval, id }));
    const { result, rerender } = renderHook(({ id }) => useApprovalReview(id), { initialProps: { id: 'one' }, wrapper });
    await waitFor(() => expect(result.current?.approval?.id).toBe('one'));
    act(() => { void result.current.approve({ persistence: 'once' }); });
    await waitFor(() => expect(result.current.submitting).toBe(true));
    expect(result.current.submittingAction).toBe('approve');
    rerender({ id: 'two' });
    await waitFor(() => expect(result.current.approval?.id).toBe('two'));
    expect(result.current.submitting).toBe(true);
    expect(result.current.submittingAction).toBeNull();
    expect(approve).toHaveBeenCalledTimes(1);
    await act(async () => { response.resolve({ id: 'one', status: 'approved', persistence: 'once', approved_at: '2026-01-01T00:01:00Z' }); });
    await waitFor(() => expect(result.current.submitting).toBe(false));
    await act(async () => { await result.current.deny(); });
    expect(deny).toHaveBeenCalledWith('two', {});
  });

  it.each([409, 410])('refreshes authoritative state after HTTP %s without retrying the decision', async (status) => {
    apiClient.defaults.adapter = async config => {
      throw new AxiosError('Decision unavailable', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: status === 409 ? 'conflict' : 'gone', message: 'Decision is no longer available' },
        status, statusText: String(status), headers: {},
      });
    };
    const deny = vi.spyOn(approvalApi, 'denyApproval');
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current?.approval?.status).toBe('pending'));
    vi.mocked(approvalApi.getApproval).mockResolvedValue({ ...approval, status: 'approved', persistence: 'session' });
    await act(async () => { await result.current.deny(); });
    await waitFor(() => expect(result.current.approval?.status).toBe('approved'));
    expect(deny).toHaveBeenCalledTimes(1);
    expect(result.current.errorCode).toBe(status === 409 ? 'ALREADY_ACTIONED' : 'EXPIRED');
    expect(result.current.denyResult).toBeNull();
  });

  it.each(['approve', 'deny'] as const)('does not let a pre-decision read overwrite a successful %s', async (action) => {
    const staleRead = Promise.withResolvers<ToolApprovalDetail>();
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current?.approval?.status).toBe('pending'));
    vi.mocked(approvalApi.getApproval).mockReturnValueOnce(staleRead.promise);
    act(() => { void result.current.refetch(); });
    await waitFor(() => expect(approvalApi.getApproval).toHaveBeenCalledTimes(2));
    vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: 'one', status: 'approved', persistence: 'once', approved_at: '2026-01-01T00:01:00Z' });
    vi.spyOn(approvalApi, 'denyApproval').mockResolvedValue({ id: 'one', status: 'denied', denied_at: '2026-01-01T00:01:00Z' });
    const status = action === 'approve' ? 'approved' : 'denied';
    try {
      await act(async () => {
        if (action === 'approve') await result.current.approve({ persistence: 'once' });
        else await result.current.deny();
      });
      await waitFor(() => expect(result.current.approval?.status).toBe(status));
      await act(async () => { staleRead.resolve(approval); });
      await waitFor(() => expect(client.isFetching({ queryKey: queryKeys.approval('alice', 'one'), exact: true })).toBe(0));
      expect(result.current.approval?.status).toBe(status);
      expect(client.getQueryData<ToolApprovalDetail>(queryKeys.approval('alice', 'one'))?.status).toBe(status);
    } finally {
      staleRead.resolve(approval);
    }
  });

  it.each([409, 410])('starts a fresh authoritative read after HTTP %s instead of joining an older pending read', async (status) => {
    const staleRead = Promise.withResolvers<ToolApprovalDetail>();
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current?.approval?.status).toBe('pending'));
    vi.mocked(approvalApi.getApproval)
      .mockReturnValueOnce(staleRead.promise)
      .mockResolvedValue({ ...approval, status: 'approved', persistence: 'session' });
    act(() => { void result.current.refetch(); });
    await waitFor(() => expect(approvalApi.getApproval).toHaveBeenCalledTimes(2));
    const deny = vi.spyOn(approvalApi, 'denyApproval').mockRejectedValue({ status, code: status === 409 ? 'CONFLICT' : 'GONE', message: 'Decision is no longer available' });
    let decision!: Promise<void>;
    act(() => { decision = result.current.deny(); });
    try {
      await waitFor(() => expect(approvalApi.getApproval).toHaveBeenCalledTimes(3));
      await act(async () => { await decision; });
      await waitFor(() => expect(result.current.approval?.status).toBe('approved'));
      await act(async () => { staleRead.resolve(approval); });
      expect(result.current.approval?.status).toBe('approved');
      expect(deny).toHaveBeenCalledTimes(1);
    } finally {
      staleRead.resolve(approval);
    }
  });

  it('retains the pending decision after failure and never automatically retries it', async () => {
    const approve = vi.spyOn(approvalApi, 'approveApproval').mockRejectedValue({ status: 503, code: 'SERVER_ERROR', message: 'Unavailable' });
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current?.approval).toEqual(approval));
    await act(async () => { await result.current.approve({ persistence: 'once' }); });
    await waitFor(() => expect(result.current.errorCode).toBe('SERVER_ERROR'));
    expect(result.current.approval?.status).toBe('pending');
    expect(result.current.approveResult).toBeNull();
    expect(approve).toHaveBeenCalledTimes(1);
  });

  it('surfaces a rejected approval pattern without recording or retrying the decision', async () => {
    let requests = 0;
    apiClient.defaults.adapter = async config => {
      requests += 1;
      throw new AxiosError('Invalid pattern', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: 'invalid_pattern', message: 'params_pattern is not allowed for once persistence' },
        status: 422, statusText: 'Unprocessable Entity', headers: {},
      });
    };
    client.setQueryData(queryKeys.pending('alice'), [approval]);
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current.approval?.status).toBe('pending'));
    await act(async () => { await result.current.approve({ persistence: 'once', params_pattern: {} }); });
    await waitFor(() => expect(result.current.errorCode).toBe('INVALID_PATTERN'));
    expect(result.current.errorMessage).toBe('params_pattern is not allowed for once persistence');
    expect(result.current.approval?.status).toBe('pending');
    expect(result.current.approveResult).toBeNull();
    expect(client.getQueryState(queryKeys.pending('alice'))?.isInvalidated).toBe(true);
    expect(requests).toBe(1);
  });

  it('does not publish a delayed approval after the authentication generation changes', async () => {
    const approved = Promise.withResolvers<ApproveResponseData>();
    vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(approved.promise);
    const { result } = renderHook(() => useApprovalReview('one'), { wrapper });
    await waitFor(() => expect(result.current.approval?.status).toBe('pending'));
    act(() => { void result.current.approve({ persistence: 'once' }); });
    await waitFor(() => expect(result.current.submitting).toBe(true));
    const generation = getAuthGeneration();
    advanceAuthGeneration();
    await act(async () => { approved.resolve({ id: 'one', status: 'approved', persistence: 'once', approved_at: '2026-01-01T00:01:00Z' }); });
    expect(getAuthGeneration()).toBe(generation + 1);
    expect(result.current.approval?.status).toBe('pending');
    expect(result.current.approveResult).toBeNull();
    expect(client.getQueryData<ToolApprovalDetail>(queryKeys.approval('alice', 'one'))?.status).toBe('pending');
  });
});
