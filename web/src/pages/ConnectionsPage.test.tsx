import { StrictMode } from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import type { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consentApi } from '@services/api/consent';
import { sessionsApi, type SessionDetail, type SessionSummary } from '@services/api/sessions';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { ConnectionsPage } from './ConnectionsPage';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn() } }));
vi.mock('@services/api/sessions', () => ({ sessionsApi: { listSessions: vi.fn(), getSessionDetails: vi.fn(), refreshSession: vi.fn(), terminateSession: vi.fn() } }));
const base: SessionSummary = { id: 'mail-session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer', scope: ['read', 'send'], initiated_at: '2026-01-01T00:00:00Z', is_expired: false, access_token_expired: false, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true };
const clients: QueryClient[] = [];
function Location() {
  const location = useLocation();
  return <span aria-label="Current location">{location.pathname + location.search}</span>;
}
function setup(route = '/sessions') {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  clients.push(client);
  return render(<StrictMode><QueryProvider client={client}><MemoryRouter initialEntries={[route]}><ConnectionsPage /><Location /></MemoryRouter></QueryProvider></StrictMode>);
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(sessionsApi.listSessions).mockResolvedValue([base]);
  vi.mocked(sessionsApi.getSessionDetails).mockResolvedValue({ session: base, dependent_agents: [{ id: 'helper', display_name: 'Mail Helper' }] });
});
afterEach(() => clients.splice(0).forEach(client => client.clear()));

describe('stored connections management', () => {
  it('shows stored provider, scope count, state and creation time without fictional fields', async () => {
    setup();
    expect(await screen.findByRole('heading', { name: 'Connections' })).toBeVisible();
    const row = (await screen.findByText('Mail')).closest('tr')!;
    expect(within(row).getByText('2 scopes')).toBeVisible();
    expect(within(row).getByText('Connected')).toBeVisible();
    expect(row.querySelector('time')).toHaveAttribute('datetime', base.initiated_at);
    expect(screen.queryByText('No connection')).not.toBeInTheDocument();
    expect(screen.queryByText('Missing scopes')).not.toBeInTheDocument();
    expect(screen.queryByText('Account')).not.toBeInTheDocument();
    expect(screen.queryByText('Last use')).not.toBeInTheDocument();
  });

  it('offers refresh only with capacity and reconnects with a same-origin sessions callback', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([
      base,
      { ...base, id: 'files-session', service_id: 'files', service_display_name: 'Files', has_refresh_token: false },
      { ...base, id: 'expired-session', service_id: 'expired', service_display_name: 'Expired Provider', is_expired: true },
    ]);
    setup();
    await screen.findByText('Mail');
    expect(screen.getAllByRole('button', { name: 'Refresh' })).toHaveLength(1);
    const reconnect = screen.getByRole('button', { name: 'Reconnect' });
    const authorizeUrl = new URL(reconnect.closest('a')!.href);
    expect(authorizeUrl.origin).toBe(window.location.origin);
    expect(authorizeUrl.pathname).toBe('/api/third-party/expired/oauth2/authorize');
    expect(authorizeUrl.searchParams.get('redirect_uri')).toBe(`${window.location.origin}/sessions`);
    expect(within(screen.getByText('Files').closest('tr')!).queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
  });

  it('reads dependencies before confirmation and never disconnects on cancel', async () => {
    setup();
    fireEvent.click(await screen.findByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByText('Mail Helper')).toBeVisible();
    expect(within(dialog).getByText(/does not revoke.*provider/i)).toBeVisible();
    expect(sessionsApi.getSessionDetails).toHaveBeenCalledWith('mail', { signal: expect.any(AbortSignal) });
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
  });

  it('requires loaded dependencies and a confirmation before server-side disconnect', async () => {
    let resolve!: (value: SessionDetail) => void;
    vi.mocked(sessionsApi.getSessionDetails).mockReturnValue(new Promise<SessionDetail>(yes => { resolve = yes; }));
    vi.mocked(sessionsApi.terminateSession).mockResolvedValue();
    setup();
    fireEvent.click(await screen.findByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeDisabled();
    await act(async () => resolve({ session: base, dependent_agents: [{ id: 'helper', display_name: 'Mail Helper' }] }));
    await within(dialog).findByText('Mail Helper');
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    await waitFor(() => expect(sessionsApi.terminateSession).toHaveBeenCalledWith('mail'));
    await waitFor(() => expect(screen.queryByText('Mail')).not.toBeInTheDocument());
    expect(await screen.findByRole('status')).toHaveTextContent('Session terminated successfully.');
  });

  it('blocks disconnect when dependencies cannot be read and permits an explicit retry', async () => {
    vi.mocked(sessionsApi.getSessionDetails).mockRejectedValueOnce(new Error('offline'));
    setup();
    fireEvent.click(await screen.findByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('dialog');
    const retry = await within(dialog).findByRole('button', { name: 'Try again' });
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeDisabled();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
    fireEvent.click(retry);
    expect(await within(dialog).findByText('Mail Helper')).toBeVisible();
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeEnabled();
  });

  it('shows retry rather than connected after a failed initial read', async () => {
    vi.mocked(sessionsApi.listSessions).mockRejectedValue(new Error('offline'));
    setup();
    const retry = await screen.findByRole('button', { name: 'Try again' });
    expect(screen.queryByText('Connected')).not.toBeInTheDocument();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([base]);
    fireEvent.click(retry);
    expect(await screen.findByText('Connected')).toBeVisible();
  });

  it('announces a successful callback, clears its URL and never mutates on replay', async () => {
    setup('/sessions?success=true');
    expect(await screen.findByRole('status')).toHaveTextContent('Successfully connected to service. You can now delegate access to agents.');
    await waitFor(() => expect(screen.getByLabelText('Current location')).toHaveTextContent(/^\/sessions$/));
    expect(sessionsApi.refreshSession).not.toHaveBeenCalled();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
  });

  it.each([
    ['access_denied', 'Ignored unsafe description', 'You denied access to the service. No tokens were stored.'],
    ['invalid_scope', 'Ignored unsafe description', 'The requested permissions are not available. Please contact support.'],
    ['expired_token', 'Ignored unsafe description', 'Your session expired. Please try again.'],
    ['invalid_callback', 'Ignored unsafe description', 'Invalid response from service. Please try again.'],
    ['invalid_state', 'Ignored unsafe description', 'Invalid request state. Please try again.'],
    ['invalid_redirect_uri', 'Ignored unsafe description', 'Invalid redirect configuration. Please contact support.'],
    ['callback_failed', '<script>alert(1)</script>', '<script>alert(1)</script>'],
    ['unknown', 'Ignored unsafe description', 'Authorization failed. Please try again.'],
  ])('preserves safe callback outcome for %s', async (code, description, expected) => {
    setup(`/sessions?error=${code}&error_description=${encodeURIComponent(description)}`);
    expect(await screen.findByRole('alert')).toHaveTextContent(expected);
    expect(document.querySelector('script')).toBeNull();
    await waitFor(() => expect(screen.getByLabelText('Current location')).toHaveTextContent(/^\/sessions$/));
    expect(sessionsApi.refreshSession).not.toHaveBeenCalled();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
  });
});
