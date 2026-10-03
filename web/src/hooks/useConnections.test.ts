import { createElement, type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { QueryClient } from '@tanstack/react-query';
import { AxiosError } from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { apiClient } from '@services/api/client';
import { sessionsApi, type SessionSummary } from '@services/api/sessions';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { useConnections, useDisconnectConnection } from './useConnections';

const session: SessionSummary = { id: 'mail-session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer', scope: ['read'], initiated_at: '2026-01-01T00:00:00Z', is_expired: false, access_token_expired: true, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true };
const adapter = apiClient.defaults.adapter;
const clients: QueryClient[] = [];
function setup() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  clients.push(client);
  return { client, wrapper: ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, children) };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(sessionsApi, 'listSessions').mockResolvedValue([session]);
});
afterEach(() => {
  clients.splice(0).forEach(client => client.clear());
  apiClient.defaults.adapter = adapter;
  vi.restoreAllMocks();
});

describe('connection authority and ownership', () => {
  it('shares one principal-scoped list without exposing another principal', async () => {
    const { client, wrapper } = setup();
    client.setQueryData(queryKeys.connections('bob'), [{ ...session, service_id: 'private' }]);
    const { result } = renderHook(() => ({ first: useConnections(), second: useConnections() }), { wrapper });
    await waitFor(() => expect(result.current.first.sessions).toEqual([session]));
    expect(result.current.second.sessions).toEqual([session]);
    expect(client.getQueryData(queryKeys.connections('alice'))).toEqual([session]);
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
    expect(sessionsApi.listSessions).toHaveBeenCalledWith({ signal: expect.any(AbortSignal) });
  });

  it('waits for refresh authority and applies the server result without replay', async () => {
    const { wrapper } = setup();
    const response = deferred<SessionSummary>();
    vi.spyOn(sessionsApi, 'refreshSession').mockReturnValue(response.promise);
    const { result } = renderHook(() => useConnections(), { wrapper });
    await waitFor(() => expect(result.current.getState('mail').action).toBe('refresh'));
    act(() => { void result.current.refresh('mail'); void result.current.refresh('mail'); });
    await waitFor(() => expect(result.current.isRefreshing('mail')).toBe(true));
    expect(result.current.getState('mail').status).toBe('needs-reauthentication');
    await act(async () => response.resolve({ ...session, access_token_expired: false }));
    await waitFor(() => expect(result.current.getState('mail').status).toBe('connected'));
    expect(sessionsApi.refreshSession).toHaveBeenCalledTimes(1);
  });

  it.each([409, 502])('keeps rejected refresh evidence across reads for status %s', async status => {
    const { wrapper } = setup();
    let requests = 0;
    apiClient.defaults.adapter = async config => {
      requests += 1;
      throw new AxiosError('Refresh failed', status === 409 ? 'ERR_BAD_REQUEST' : 'ERR_BAD_RESPONSE', config, undefined, {
        config, data: {
          error: status === 409 ? 'refresh_unavailable' : 'refresh_failed',
          message: status === 409
            ? 'no valid refresh token available for this session'
            : 'failed to refresh token with the third-party provider',
        },
        status, statusText: String(status), headers: {},
      });
    };
    const { result } = renderHook(() => useConnections(), { wrapper });
    await waitFor(() => expect(result.current.sessions).toEqual([session]));
    await act(async () => result.current.refresh('mail'));
    expect(result.current.getState('mail')).toMatchObject({ status: 'needs-reauthentication', action: 'reconnect', reason: 'refresh-rejected', rejectedRefresh: true });
    expect(result.current.refreshError).toMatchObject({ status, code: status === 409 ? 'CONFLICT' : 'SERVER_ERROR' });
    await act(async () => result.current.refetch());
    expect(result.current.getState('mail').action).toBe('reconnect');
    expect(requests).toBe(1);
  });

  it('refetches after a missing refresh session rather than inventing a connection', async () => {
    const { wrapper } = setup();
    apiClient.defaults.adapter = async config => {
      throw new AxiosError('Not found', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: 'not_found', message: 'session not found' },
        status: 404, statusText: 'Not Found', headers: {},
      });
    };
    const { result } = renderHook(() => useConnections(), { wrapper });
    await waitFor(() => expect(result.current.sessions).toEqual([session]));
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
    await act(async () => result.current.refresh('mail'));
    await waitFor(() => expect(result.current.sessions).toEqual([]));
    expect(result.current.refreshError).toMatchObject({ status: 404, code: 'NOT_FOUND' });
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(2);
  });

  it('retains prior truth as stale after a network failure', async () => {
    const { wrapper } = setup();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...session, access_token_expired: false }]);
    apiClient.defaults.adapter = async config => { throw new AxiosError('offline', 'ERR_NETWORK', config); };
    const { result } = renderHook(() => useConnections(), { wrapper });
    await waitFor(() => expect(result.current.getState('mail').status).toBe('connected'));
    await act(async () => result.current.refresh('mail'));
    expect(result.current.getState('mail')).toMatchObject({ status: 'connected', stale: true });
    expect(result.current.refreshError).toMatchObject({ status: 0, code: 'NETWORK_ERROR' });
    expect(result.current.sessions).toHaveLength(1);
  });

  it('keeps loaded rows stale with retry after a failed list reread', async () => {
    const { wrapper } = setup();
    const { result } = renderHook(() => useConnections(), { wrapper });
    await waitFor(() => expect(result.current.sessions).toEqual([session]));
    vi.mocked(sessionsApi.listSessions).mockRejectedValue(new Error('offline'));
    await act(async () => result.current.refetch());
    await waitFor(() => expect(result.current.getState('mail')).toMatchObject({
      status: 'needs-reauthentication', stale: true, action: 'retry',
    }));
    expect(result.current.sessions).toEqual([session]);
  });

  it('confirms disconnect and keeps independent pending rows through unrelated failures', async () => {
    const { wrapper } = setup();
    const other = { ...session, service_id: 'calendar', id: 'calendar-session' };
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([session, other]);
    const mail = deferred<void>();
    const calendar = deferred<void>();
    vi.spyOn(sessionsApi, 'terminateSession').mockImplementation(id => id === 'mail' ? mail.promise : calendar.promise);
    const success = vi.fn();
    const { result } = renderHook(() => ({ read: useConnections(), revoke: useDisconnectConnection({ onSuccess: success }) }), { wrapper });
    await waitFor(() => expect(result.current.read.sessions).toEqual([session, other]));
    act(() => result.current.revoke.requestRevoke(session));
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
    act(() => result.current.revoke.cancelRevoke());
    expect(result.current.read.sessions).toEqual([session, other]);
    act(() => result.current.revoke.requestRevoke(session));
    act(() => { void result.current.revoke.confirmRevoke(); });
    act(() => result.current.revoke.requestRevoke(other));
    act(() => { void result.current.revoke.confirmRevoke(); });
    await waitFor(() => expect(result.current.revoke.isPending('mail')).toBe(true));
    expect(success).not.toHaveBeenCalled();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([session]);
    await act(async () => calendar.resolve());
    await waitFor(() => expect(result.current.read.sessions).toEqual([session]));
    expect(result.current.revoke.isPending('mail')).toBe(true);
    await act(async () => mail.reject(new Error('offline')));
    await waitFor(() => expect(result.current.revoke.error).toBeInstanceOf(Error));
    expect(result.current.read.sessions).toEqual([session]);
    expect(success).toHaveBeenCalledTimes(1);
    expect(success).toHaveBeenCalledWith('calendar');
  });

  it('keeps newest-first connections stable when a reread changes source order', async () => {
    const { wrapper } = setup();
    const first = { ...session, service_id: 'first' };
    const second = { ...session, service_id: 'second', initiated_at: '2025-12-31T19:00:00-05:00' };
    const alphabet = { ...session, service_id: 'alphabet', service_display_name: 'Alphabet' };
    const newest = { ...session, service_id: 'newest', initiated_at: '2026-01-02T00:00:00Z' };
    vi.mocked(sessionsApi.listSessions)
      .mockResolvedValueOnce([newest, first, alphabet, second])
      .mockResolvedValueOnce([first, second, newest, alphabet]);
    const { result } = renderHook(() => useConnections(), { wrapper });
    await waitFor(() => expect(result.current.sessions.map(item => item.service_id)).toEqual(['newest', 'alphabet', 'second', 'first']));
    await act(async () => { await result.current.refetch(); });
    expect(result.current.sessions.map(item => item.service_id)).toEqual(['newest', 'alphabet', 'second', 'first']);
  });
});
