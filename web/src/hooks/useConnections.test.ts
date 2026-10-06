import { createElement, type ReactNode } from 'react';
import { act, cleanup, fireEvent, render, renderHook, screen, waitFor, within } from '@testing-library/react';
import type { QueryClient } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { AxiosError } from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { apiClient } from '@services/api/client';
import { sessionsApi, type SessionSummary } from '@services/api/sessions';
import { QueryProvider } from '@services/query/QueryProvider';
import { ConnectionCard } from '@components/sessions/ConnectionCard';
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
  return { client, wrapper: ({ children }: { children: ReactNode }) => createElement(QueryProvider, { client }, createElement(MemoryRouter, { initialEntries: ['/connections'] }, children)) };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
async function loadConnectionQueries() {
  // Commit the identity boundary before delivering the newly mounted list query.
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
}
function ConnectionCards({ view }: { view: 'grid' | 'list' }) {
  const connections = useConnections();
  return createElement('div', null, connections.sessions.map(record => createElement(ConnectionCard, {
    key: record.service_id, session: record, state: connections.getState(record.service_id), view,
    refreshing: connections.isRefreshing(record.service_id), disconnecting: false,
    onRefresh: serviceId => { void connections.refresh(serviceId); },
    onDisconnect: () => undefined, onRetry: () => { void connections.refetch(); },
  })));
}
beforeEach(() => {
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(sessionsApi, 'listSessions').mockResolvedValue([session]);
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach(client => client.clear());
  apiClient.defaults.adapter = adapter;
  vi.restoreAllMocks();
  vi.useRealTimers();
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
    const onRefreshSuccess = vi.fn();
    const { result } = renderHook(() => useConnections({ onRefreshSuccess }), { wrapper });
    await waitFor(() => expect(result.current.getState('mail').action).toBe('refresh'));
    act(() => { void result.current.refresh('mail'); void result.current.refresh('mail'); });
    await waitFor(() => expect(result.current.isRefreshing('mail')).toBe(true));
    expect(result.current.getState('mail').status).toBe('needs-reauthentication');
    await act(async () => response.resolve({ ...session, access_token_expired: false }));
    await waitFor(() => expect(result.current.getState('mail').status).toBe('connected'));
    expect(sessionsApi.refreshSession).toHaveBeenCalledTimes(1);
    expect(onRefreshSuccess).toHaveBeenCalledTimes(1);
    expect(onRefreshSuccess).toHaveBeenCalledWith('mail');
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
    const onRefreshError = vi.fn();
    const { result } = renderHook(() => useConnections({ onRefreshError }), { wrapper });
    await waitFor(() => expect(result.current.getState('mail').status).toBe('connected'));
    await act(async () => result.current.refresh('mail'));
    expect(result.current.getState('mail')).toMatchObject({ status: 'connected', stale: true });
    expect(result.current.refreshError).toMatchObject({ status: 0, code: 'NETWORK_ERROR' });
    expect(result.current.sessions).toHaveLength(1);
    expect(onRefreshError).toHaveBeenCalledTimes(1);
    expect(onRefreshError).toHaveBeenCalledWith('mail');
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

describe('connection deadline updates', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    vi.setSystemTime(new Date('2026-10-05T12:00:00Z'));
  });

  it.each(['grid', 'list'] as const)('replaces visible Connected with Expired and Reconnect at the deadline in %s view', async view => {
    const deadline = Date.now() + 10_000;
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...session, access_token_expired: false, refresh_token_expires_at: new Date(deadline).toISOString() }]);
    const refresh = vi.spyOn(sessionsApi, 'refreshSession');
    const { wrapper } = setup();
    render(createElement(ConnectionCards, { view }), { wrapper });
    await loadConnectionQueries();
    const status = within(screen.getByTestId(view === 'grid' ? 'connection-card' : 'connection-row')).getByTestId('connection-state');
    expect(status).toHaveTextContent(/^Connected$/);
    expect(status).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Reconnect', exact: true })).not.toBeInTheDocument();
    await act(async () => { await vi.advanceTimersByTimeAsync(deadline - Date.now() - 1); });
    expect(status).toHaveTextContent(/^Connected$/);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(status).toHaveTextContent(/^Expired$/);
    expect(status).toBeVisible();
    expect(screen.getByRole('button', { name: 'Reconnect', exact: true })).toHaveAttribute('href',
      `/api/third-party/mail/oauth2/authorize?redirect_uri=${encodeURIComponent(`${window.location.origin}/connections`)}`);
    expect(screen.queryByRole('button', { name: 'Refresh', exact: true })).not.toBeInTheDocument();
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('replaces a stale Refresh click with Reconnect without submitting expired credentials', async () => {
    const deadline = Date.now() + 10_000;
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...session, refresh_token_expires_at: new Date(deadline).toISOString() }]);
    const refresh = vi.spyOn(sessionsApi, 'refreshSession');
    const { wrapper } = setup();
    render(createElement(ConnectionCards, { view: 'grid' }), { wrapper });
    await loadConnectionQueries();
    const button = screen.getByRole('button', { name: 'Refresh', exact: true });
    // Simulate a delayed browser timer: wall time passes before its callback runs.
    vi.setSystemTime(deadline);
    fireEvent.click(button);
    expect(screen.queryByRole('button', { name: 'Refresh', exact: true })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reconnect', exact: true })).toBeVisible();
    expect(screen.getByText('Expired', { exact: true })).toBeVisible();
    expect(refresh).not.toHaveBeenCalled();
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
  });

  it('updates each connection at its own deadline without a list fetch', async () => {
    const firstDeadline = Date.now() + 10_000;
    const secondDeadline = firstDeadline + 10_000;
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([
      { ...session, access_token_expired: false, refresh_token_expires_at: new Date(firstDeadline).toISOString() },
      { ...session, service_id: 'calendar', access_token_expired: false, refresh_token_expires_at: new Date(secondDeadline).toISOString() },
    ]);
    const { wrapper } = setup();
    const { result } = renderHook(() => {
      const connections = useConnections();
      // Capture during render; calling getState later would hide a missing rerender.
      return [connections.getState('mail').status, connections.getState('calendar').status];
    }, { wrapper });
    await loadConnectionQueries();
    expect(result.current).toEqual(['connected', 'connected']);
    await act(async () => { await vi.advanceTimersByTimeAsync(firstDeadline - Date.now()); });
    expect(result.current).toEqual(['expired', 'connected']);
    await act(async () => { await vi.advanceTimersByTimeAsync(secondDeadline - Date.now()); });
    expect(result.current).toEqual(['expired', 'expired']);
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
  });

  it('cancels the previous deadline when a refreshed response extends token lifetime', async () => {
    const originalDeadline = Date.now() + 10_000;
    const refreshedDeadline = originalDeadline + 20_000;
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...session, refresh_token_expires_at: new Date(originalDeadline).toISOString() }]);
    const response = deferred<SessionSummary>();
    vi.spyOn(sessionsApi, 'refreshSession').mockReturnValue(response.promise);
    const { wrapper } = setup();
    const { result } = renderHook(() => {
      const connections = useConnections();
      return { connections, state: connections.getState('mail') };
    }, { wrapper });
    await loadConnectionQueries();
    act(() => { void result.current.connections.refresh('mail'); });
    await act(async () => { await vi.advanceTimersByTimeAsync(1_000); });
    await act(async () => response.resolve({ ...session, access_token_expired: false, refresh_token_expires_at: new Date(refreshedDeadline).toISOString() }));
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current.state).toMatchObject({ status: 'connected', action: 'refresh' });
    const beforeOldDeadline = result.current;
    await act(async () => { await vi.advanceTimersByTimeAsync(originalDeadline - Date.now()); });
    expect(result.current).toBe(beforeOldDeadline);
    await act(async () => { await vi.advanceTimersByTimeAsync(refreshedDeadline - Date.now() - 1); });
    expect(result.current.state.status).toBe('connected');
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current.state).toMatchObject({ status: 'expired', action: 'reconnect' });
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
    expect(sessionsApi.refreshSession).toHaveBeenCalledTimes(1);
  });

  it('clears deadline work when the input list changes or the hook unmounts', async () => {
    const deadline = Date.now() + 10_000;
    const record = { ...session, access_token_expired: false, refresh_token_expires_at: new Date(deadline).toISOString() };
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([record]);
    const timers = vi.spyOn(window, 'setTimeout');
    const clearTimer = vi.spyOn(window, 'clearTimeout');
    const renders = vi.fn();
    const { client, wrapper } = setup();
    const { result, unmount } = renderHook(() => {
      renders();
      const connections = useConnections();
      return { connections, state: connections.getState('mail') };
    }, { wrapper });
    await loadConnectionQueries();
    const timerIndex = timers.mock.calls.findIndex(([, delay]) => Number(delay) >= 9_998 && Number(delay) <= 10_000);
    expect(timerIndex).toBeGreaterThanOrEqual(0);
    const timer = timers.mock.results[timerIndex]!.value;
    act(() => client.setQueryData(queryKeys.connections('alice'), []));
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current.connections.sessions).toEqual([]);
    expect(clearTimer).toHaveBeenCalledWith(timer);
    renders.mockClear();
    await act(async () => { await vi.advanceTimersByTimeAsync(deadline - Date.now()); });
    expect(renders).not.toHaveBeenCalled();
    timers.mockClear();
    act(() => client.setQueryData(queryKeys.connections('alice'), [{ ...record, refresh_token_expires_at: new Date(Date.now() + 10_000).toISOString() }]));
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    const mountedTimerIndex = timers.mock.calls.findIndex(([, delay]) => Number(delay) >= 9_998 && Number(delay) <= 10_000);
    expect(mountedTimerIndex).toBeGreaterThanOrEqual(0);
    const mountedTimer = timers.mock.results[mountedTimerIndex]!.value;
    unmount();
    expect(clearTimer).toHaveBeenCalledWith(mountedTimer);
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
  });

  it('splits distant deadlines into browser-safe timer delays and still expires on time', async () => {
    const maximumDelay = 2_147_483_647;
    const deadline = Date.now() + maximumDelay + 10_000;
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...session, access_token_expired: false, refresh_token_expires_at: new Date(deadline).toISOString() }]);
    const timers = vi.spyOn(window, 'setTimeout');
    const { wrapper } = setup();
    const { result } = renderHook(() => {
      const connections = useConnections();
      return connections.getState('mail');
    }, { wrapper });
    await loadConnectionQueries();
    expect(timers.mock.calls.some(([, delay]) => delay === maximumDelay)).toBe(true);
    expect(timers.mock.calls.every(([, delay]) => Number(delay ?? 0) <= maximumDelay)).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(maximumDelay); });
    expect(result.current.status).toBe('connected');
    await act(async () => { await vi.advanceTimersByTimeAsync(deadline - Date.now() - 1); });
    expect(result.current.status).toBe('connected');
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(result.current).toMatchObject({ status: 'expired', action: 'reconnect' });
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
  });

  it.each([
    { has_refresh_token: true, refresh_token_expires_at: undefined, status: 'connected' },
    { has_refresh_token: true, refresh_token_expires_at: 'invalid', status: 'connected' },
    { has_refresh_token: false, refresh_token_expires_at: '2026-10-05T12:00:01Z', status: 'connected' },
    { has_refresh_token: true, refresh_token_expires_at: '2026-10-05T11:59:59Z', status: 'expired' },
  ])('keeps existing deadline semantics for $has_refresh_token / $refresh_token_expires_at', async ({ status, ...fields }) => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...session, access_token_expired: false, ...fields }]);
    const { wrapper } = setup();
    const { result } = renderHook(() => {
      const connections = useConnections();
      return connections.getState('mail');
    }, { wrapper });
    await loadConnectionQueries();
    expect(result.current.status).toBe(status);
    const before = result.current;
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(result.current).toBe(before);
    expect(sessionsApi.listSessions).toHaveBeenCalledTimes(1);
  });
});
