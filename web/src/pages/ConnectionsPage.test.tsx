import { StrictMode } from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import type { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { toast } from 'sonner';
import { consentApi } from '@services/api/consent';
import { sessionsApi, type AffectedAgent, type SessionSummary } from '@services/api/sessions';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { Toaster } from '@design-system/components/feedback/Toaster';
import { ConnectionsPage } from './ConnectionsPage';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn() } }));
vi.mock('@services/api/sessions', () => ({ sessionsApi: { listSessions: vi.fn(), getAffectedAgents: vi.fn(), refreshSession: vi.fn(), terminateSession: vi.fn() } }));
const base: SessionSummary = { id: 'mail-session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer', scope: ['read', 'send'], initiated_at: '2026-01-01T00:00:00Z', is_expired: false, access_token_expired: false, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true };
const affectedAgents: AffectedAgent[] = [{ agent_id: 'helper', display_name: 'Mail Helper' }];
const clients: QueryClient[] = [];
function Location() {
  const location = useLocation();
  return <span aria-label="Current location">{location.pathname + location.search}</span>;
}
function setup(route = '/connections') {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  clients.push(client);
  return render(<StrictMode><ThemeProvider><QueryProvider client={client}><MemoryRouter initialEntries={[route]}><ConnectionsPage /><Location /><Toaster label="Notifications" closeButtonLabel="Close" /></MemoryRouter></QueryProvider></ThemeProvider></StrictMode>);
}
beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(sessionsApi.listSessions).mockResolvedValue([base]);
  vi.mocked(sessionsApi.getAffectedAgents).mockResolvedValue(affectedAgents);
});
afterEach(() => { toast.dismiss(); vi.unstubAllGlobals(); localStorage.clear(); clients.splice(0).forEach(client => client.clear()); });

describe('stored connections management', () => {
  it('shows provider, interactive raw scopes, state and initiated date without fictional fields', async () => {
    const user = userEvent.setup();
    setup();
    expect(await screen.findByRole('heading', { name: 'Connections' })).toBeVisible();
    const card = await screen.findByTestId('connection-card');
    expect(within(card).getByText('Mail')).toBeVisible();
    expect(within(card).getByRole('button', { name: '2 scopes' })).toBeVisible();
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Connected');
    expect(within(card).getByTestId('connection-created-at')).toHaveAttribute('datetime', base.initiated_at);
    await user.click(within(card).getByRole('button', { name: '2 scopes' }));
    expect(await screen.findByText('read')).toHaveClass('font-mono');
    expect(screen.getByText('send')).toBeVisible();
    expect(screen.queryByText('No connection')).not.toBeInTheDocument();
    expect(screen.queryByText('Missing scopes')).not.toBeInTheDocument();
    expect(screen.queryByText('Account')).not.toBeInTheDocument();
    expect(screen.queryByText('Last use')).not.toBeInTheDocument();
  });

  it.each(['grid', 'list'] as const)('reveals needs-sign-in guidance on hover and keyboard focus in %s view', async (view) => {
    const user = userEvent.setup();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...base, access_token_expired: true }]);
    localStorage.setItem('aib.collection-view.connections', view);
    setup();
    const badge = await screen.findByRole('button', { name: 'Needs sign-in' });
    expect(screen.queryByText('Your access expired. Refresh to continue.')).not.toBeInTheDocument();
    await user.hover(badge);
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Your access expired. Refresh to continue.');
    await user.unhover(badge);
    fireEvent.pointerMove(document.body, { clientX: 500, clientY: 500 });
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
    screen.getByRole('button', { name: 'List view' }).focus();
    await user.tab();
    expect(badge).toHaveFocus();
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Your access expired. Refresh to continue.');
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
    expect(within(screen.getByTestId(view === 'grid' ? 'connection-card' : 'connection-row')).getByRole('button', { name: 'Refresh' })).toBeVisible();
    expect(sessionsApi.refreshSession).not.toHaveBeenCalled();
  });

  it('offers refresh only when access requires it and reconnects with a same-origin connections callback', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([
      base,
      { ...base, id: 'calendar-connection', service_id: 'calendar', service_display_name: 'Calendar', access_token_expired: true },
      { ...base, id: 'files-connection', service_id: 'files', service_display_name: 'Files', has_refresh_token: false },
      { ...base, id: 'expired-connection', service_id: 'expired', service_display_name: 'Expired Provider', is_expired: true },
    ]);
    setup();
    await screen.findByText('Mail');
    expect(screen.getAllByRole('button', { name: 'Refresh' })).toHaveLength(1);
    expect(within(screen.getByText('Mail').closest('[data-slot="entity-card"]')!).queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
    const reconnect = screen.getByRole('button', { name: 'Reconnect' });
    const authorizeUrl = new URL(reconnect.closest('a')!.href);
    expect(authorizeUrl.origin).toBe(window.location.origin);
    expect(authorizeUrl.pathname).toBe('/api/third-party/expired/oauth2/authorize');
    expect(authorizeUrl.searchParams.get('redirect_uri')).toBe(`${window.location.origin}/connections`);
    expect(within(screen.getByText('Files').closest('[data-slot="entity-card"]')!).queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
    for (const card of screen.getAllByTestId('connection-card')) {
      expect(within(card).getByTestId('connection-scope-count')).toBeVisible();
      expect(within(card).getByTestId('connection-created-at')).toHaveAttribute('datetime', base.initiated_at);
    }
  });

  it('sorts attention first, links search and state filters, and persists the list choice', async () => {
    const user = userEvent.setup();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([
      base,
      { ...base, id: 'calendar-connection', service_id: 'calendar', service_display_name: 'Calendar', access_token_expired: true, initiated_at: '2025-12-01T00:00:00Z' },
      { ...base, id: 'files-connection', service_id: 'files', service_display_name: 'Files', is_expired: true, initiated_at: '2025-11-01T00:00:00Z' },
    ]);
    const page = setup();
    await screen.findByText('Files');
    expect(screen.getAllByTestId('connection-card').map(card => card.id)).toEqual(['connection-files-connection', 'connection-calendar-connection', 'connection-mail-session']);
    await user.click(screen.getByRole('button', { name: 'Sort' }));
    await user.click(screen.getByRole('menuitemradio', { name: 'Recently connected' }));
    expect(screen.getAllByTestId('connection-card').map(card => card.id)).toEqual(['connection-mail-session', 'connection-calendar-connection', 'connection-files-connection']);
    expect(screen.getByLabelText('Current location')).toHaveTextContent('sort=recent');
    await user.click(screen.getByRole('button', { name: 'Filter' }));
    await user.click(within(screen.getByRole('group', { name: 'Filter' })).getByRole('button', { name: 'Expired' }));
    await waitFor(() => expect(screen.getAllByTestId('connection-card')).toHaveLength(1));
    expect(screen.getByLabelText('Current location')).toHaveTextContent('state=expired');
    await user.click(screen.getByRole('button', { name: 'Search', exact: true }));
    await user.type(screen.getByRole('textbox', { name: 'Search' }), 'file');
    expect(screen.getByLabelText('Current location')).toHaveTextContent('q=file');
    await user.click(screen.getByRole('button', { name: 'List view' }));
    expect(screen.queryByRole('textbox', { name: 'Search' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search: file' })).toBeVisible();
    expect(screen.getByTestId('connection-row')).toBeVisible();
    expect(localStorage.getItem('aib.collection-view.connections')).toBe('list');
    await user.click(within(screen.getByLabelText('Active connection filters')).getByRole('button', { name: /Filter: Expired/ }));
    expect(screen.getByLabelText('Current location')).not.toHaveTextContent('state=expired');
    expect(screen.getByTestId('connection-row')).toBeVisible();
    expect(screen.getByLabelText('Current location')).toHaveTextContent('q=file');
    page.unmount();
    setup('/connections?q=file');
    expect(await screen.findByTestId('connection-row')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Search: file', exact: true }));
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveValue('file');
  });

  it('preserves typed results and URL state on outside blur and restores the query with slash', async () => {
    const user = userEvent.setup();
    setup();
    await screen.findByTestId('connection-card');
    await user.click(screen.getByRole('button', { name: 'Search', exact: true }));
    await user.type(screen.getByRole('textbox', { name: 'Search' }), 'Mail');
    await user.click(screen.getByRole('button', { name: 'List view' }));
    expect(screen.queryByRole('textbox', { name: 'Search' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'List view' })).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Search: Mail' })).toBeVisible();
    expect(screen.getByLabelText('Current location')).toHaveTextContent('q=Mail');
    expect(screen.getByTestId('connection-row')).toHaveTextContent('Mail');
    await user.keyboard('/');
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveValue('Mail');
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveFocus();
    expect(screen.getByLabelText('Current location')).toHaveTextContent('q=Mail');
  });

  it('clears a no-results search without clearing the state filter', async () => {
    const user = userEvent.setup();
    setup('/connections?q=missing&state=connected');
    const clearSearch = await waitFor(() => within(screen.getByRole('status')).getByRole('button', { name: 'Clear search' }));
    await user.click(clearSearch);
    expect(await screen.findByTestId('connection-card')).toHaveTextContent('Mail');
    expect(screen.getByLabelText('Current location')).not.toHaveTextContent('q=');
    expect(screen.getByLabelText('Current location')).toHaveTextContent('state=connected');
    expect(within(screen.getByLabelText('Active connection filters')).getAllByRole('button')).toHaveLength(1);
  });

  it('lets an empty state filter be cleared without changing the sort', async () => {
    const user = userEvent.setup();
    setup('/connections?state=expired&sort=name');
    const clearFilter = await waitFor(() => within(screen.getByRole('status')).getByRole('button', { name: 'Clear filter' }));
    await user.click(clearFilter);
    expect(await screen.findByTestId('connection-card')).toHaveTextContent('Mail');
    expect(screen.getByLabelText('Current location')).not.toHaveTextContent('state=');
    expect(screen.getByLabelText('Current location')).toHaveTextContent('sort=name');
  });

  it('refreshes a needs-sign-in connection only after server acceptance, then pulses its card', async () => {
    const user = userEvent.setup();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ ...base, access_token_expired: true }]);
    let accept!: (value: SessionSummary) => void;
    vi.mocked(sessionsApi.refreshSession).mockReturnValue(new Promise<SessionSummary>(resolve => { accept = resolve; }));
    setup();
    const card = await screen.findByTestId('connection-card');
    expect(within(card).getByText('Needs sign-in')).toBeVisible();
    await user.click(within(card).getByRole('button', { name: 'Refresh' }));
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Needs sign-in');
    expect(screen.queryByText('Connection refreshed.')).not.toBeInTheDocument();
    await act(async () => accept({ ...base, scope: ['read', 'send', 'write'] }));
    expect(await screen.findByText('Connection refreshed.')).toBeVisible();
    expect(within(screen.getByTestId('connection-card')).getByTestId('connection-state')).toHaveTextContent('Connected');
    expect(screen.getByTestId('connection-card')).toHaveClass('connection-just-updated');
    expect(within(screen.getByTestId('connection-card')).getByRole('button', { name: '3 scopes' })).toBeVisible();
  });

  it('shows unavailable and retry while keeping the last known metadata on a failed reread', async () => {
    const user = userEvent.setup();
    setup();
    await screen.findByTestId('connection-state');
    vi.mocked(sessionsApi.listSessions).mockRejectedValue(new Error('offline'));
    await act(async () => { await clients.at(-1)!.refetchQueries({ queryKey: queryKeys.connections('alice'), exact: true }); });
    const card = screen.getByTestId('connection-card');
    await waitFor(() => expect(within(card).getByText('Unavailable')).toBeVisible());
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Unavailable');
    expect(within(card).getByTestId('connection-scope-count')).toBeVisible();
    expect(within(card).getByTestId('connection-created-at')).toHaveAttribute('datetime', base.initiated_at);
    expect(within(card).getByRole('button', { name: 'Try again' })).toBeVisible();
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([base]);
    await user.click(within(card).getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(screen.getByTestId('connection-state')).toHaveTextContent('Connected'));
  });

  it('reads dependencies before confirmation and never disconnects on cancel', async () => {
    setup();
    const trigger = await screen.findByRole('button', { name: 'Disconnect' });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = await screen.findByRole('dialog');
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus());
    expect(await within(dialog).findByText('Mail Helper')).toBeVisible();
    expect(within(dialog).getByText(/does not revoke.*provider/i)).toBeVisible();
    expect(sessionsApi.getAffectedAgents).toHaveBeenCalledWith('mail', { signal: expect.any(AbortSignal) });
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Disconnect' })).toHaveFocus());
  });

  it('requires loaded dependencies and a confirmation before server-side disconnect', async () => {
    let resolve!: (value: AffectedAgent[]) => void;
    vi.mocked(sessionsApi.getAffectedAgents).mockReturnValue(new Promise<AffectedAgent[]>(yes => { resolve = yes; }));
    vi.mocked(sessionsApi.terminateSession).mockResolvedValue();
    setup();
    const trigger = await screen.findByRole('button', { name: 'Disconnect' });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeDisabled();
    await act(async () => resolve(affectedAgents));
    await within(dialog).findByText('Mail Helper');
    vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    await waitFor(() => expect(sessionsApi.terminateSession).toHaveBeenCalledWith('mail'));
    await waitFor(() => expect(screen.queryByText('Mail')).not.toBeInTheDocument());
    expect(await screen.findByText('Connection disconnected.')).toBeVisible();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Connections' }).closest('[tabindex="-1"]')).toHaveFocus());
  });

  it('keeps the connection after a rejected disconnect and shows only safe local copy', async () => {
    const user = userEvent.setup();
    vi.mocked(sessionsApi.terminateSession).mockRejectedValue(new Error('Session token is unavailable'));
    setup();
    await user.click(await screen.findByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByText('Mail Helper');
    await user.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    expect(await screen.findByText('Could not disconnect this connection. Please try again.')).toBeVisible();
    expect(screen.getByTestId('connection-card')).toBeVisible();
    expect(screen.queryByText('Session token is unavailable')).not.toBeInTheDocument();
  });

  it('blocks disconnect when dependencies cannot be read and permits an explicit retry', async () => {
    vi.mocked(sessionsApi.getAffectedAgents).mockRejectedValueOnce(new Error('offline'));
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
    await waitFor(() => expect(screen.getByTestId('connection-state')).toHaveTextContent('Connected'));
  });

  it('consumes a successful callback once and never mutates credentials on replay', async () => {
    setup('/connections?success=true&service_id=mail');
    expect(await screen.findByText('Service connected. Your agents can now request access.')).toBeVisible();
    await waitFor(() => expect(screen.getByLabelText('Current location')).toHaveTextContent(/^\/connections$/));
    expect(sessionsApi.refreshSession).not.toHaveBeenCalled();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
  });

  it.each([
    ['access_denied', 'Ignored unsafe description', 'You denied access to the service. No tokens were stored.'],
    ['invalid_scope', 'Ignored unsafe description', 'The requested permissions are not available. Please contact support.'],
    ['expired_token', 'Ignored unsafe description', 'Your authorization expired. Please try again.'],
    ['invalid_callback', 'Ignored unsafe description', 'Invalid response from service. Please try again.'],
    ['invalid_state', 'Ignored unsafe description', 'Invalid request state. Please try again.'],
    ['invalid_redirect_uri', 'Ignored unsafe description', 'Invalid redirect configuration. Please contact support.'],
    ['callback_failed', '<script>alert(1)</script>', 'Authorization failed. Please try again.'],
    ['unknown', 'Ignored unsafe description', 'Authorization failed. Please try again.'],
  ])('uses safe callback copy for %s without exposing the server description', async (code, description, expected) => {
    setup(`/connections?error=${code}&error_description=${encodeURIComponent(description)}`);
    expect(await screen.findByText(expected)).toBeVisible();
    expect(document.querySelector('script')).toBeNull();
    expect(screen.queryByText(description)).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText('Current location')).toHaveTextContent(/^\/connections$/));
    expect(sessionsApi.refreshSession).not.toHaveBeenCalled();
    expect(sessionsApi.terminateSession).not.toHaveBeenCalled();
  });
});
