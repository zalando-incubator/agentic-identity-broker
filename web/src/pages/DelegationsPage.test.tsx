import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { Toaster } from '@design-system/components/feedback/Toaster';
import { toast } from 'sonner';
import type { AgentDelegation } from '../types/consent';
import { DelegationsPage } from './DelegationsPage';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDelegations: vi.fn(), deleteGrant: vi.fn() } }));
const rows: AgentDelegation[] = [{ agentId: 'alpha', displayName: '<img src=x onerror=alert(1)> Alpha', activeGrantCount: 435, lastModifiedAt: '2026-01-01T00:00:00Z' }, { agentId: 'beta', displayName: 'Beta', activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z', expiresAt: '2099-01-01T00:00:00Z' }];
const clients: QueryClient[] = [];
function SearchLocation() { const location = useLocation(); return <><output data-testid="agents-query">{location.search}</output><span data-testid="agents-path">{location.pathname}</span></>; }
function setup(entry = '/agents') {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } }); clients.push(client);
  return render(<ThemeProvider><QueryProvider client={client}><MemoryRouter initialEntries={[entry]}><SearchLocation /><DelegationsPage /><Toaster label="Notifications" closeButtonLabel="Close" /></MemoryRouter></QueryProvider></ThemeProvider>);
}
beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDelegations).mockResolvedValue(rows);
});
afterEach(() => { toast.dismiss(); vi.unstubAllGlobals(); vi.useRealTimers(); clients.splice(0).forEach((client) => client.clear()); });

describe('Agents console', () => {
  it('shows linked agent cards without counts, escapes names, and filters case-insensitively', async () => {
    const user = userEvent.setup(); const { container } = setup();
    const entities = await screen.findAllByTestId('agent-entity');
    expect(screen.getByRole('heading', { name: 'Agents', level: 1 })).toBeVisible();
    expect(screen.queryByText('Review and revoke the access you have delegated to agents.')).not.toBeInTheDocument();
    expect(entities).toHaveLength(2);
    expect(within(entities[0]!).getByRole('link', { name: rows[0]!.displayName })).toHaveAttribute('href', '/agents/alpha');
    expect(within(entities[0]!).getByTestId('agent-expiry')).toHaveTextContent('Until revoked');
    const changed = within(entities[0]!).getByTestId('agent-changed-at');
    expect(changed).toHaveAttribute('dateTime', rows[0]!.lastModifiedAt);
    expect(changed).toHaveAttribute('title', new Date(rows[0]!.lastModifiedAt).toLocaleString());
    expect(changed).toHaveTextContent('ago');
    expect(entities[0]).not.toHaveTextContent('435');
    expect(container.querySelector('img[src="x"]')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Status' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    await user.type(screen.getByRole('textbox', { name: 'Search agents' }), 'bETA');
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(1));
    expect(screen.getByRole('link', { name: 'Beta' })).toBeVisible();
    expect(screen.getByTestId('agents-query')).toHaveTextContent('q=bETA');
  });

  it('keeps the filtered agents and URL query when search closes and restores its value on reopening', async () => {
    const user = userEvent.setup(); setup(); await screen.findByRole('link', { name: 'Beta' });
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    await user.type(screen.getByRole('textbox', { name: 'Search agents' }), 'Beta');
    await user.click(screen.getByRole('button', { name: 'List view' }));
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'List view' })).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Search agents: Beta' })).toBeVisible();
    expect(screen.getByTestId('agents-query')).toHaveTextContent('q=Beta');
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(1));
    expect(screen.getByRole('link', { name: 'Beta' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Search agents: Beta' }));
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveValue('Beta');
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveFocus();
  });

  it.each(['grid', 'list'])('keeps one detail link and a separate keyboard-operable Revoke in %s view', async view => {
    const user = userEvent.setup(); setup(); await screen.findByRole('link', { name: 'Beta' });
    if (view === 'list') await user.click(screen.getByRole('button', { name: 'List view' }));
    const entity = screen.getByText('Beta').closest('[data-testid="agent-entity"]')!;
    const surface = within(entity).getByRole('link', { name: 'Beta' });
    const revoke = within(entity).getByRole('button', { name: 'Revoke' });
    expect(within(entity).getAllByRole('link')).toEqual([surface]);
    expect(surface).not.toContainElement(revoke);
    surface.focus();
    await user.tab();
    expect(revoke).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('dialog', { name: 'Revoke access' })).toBeVisible();
    expect(screen.getByTestId('agents-path')).toHaveTextContent('/agents');
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
  });

  it.each(['grid', 'list'])('navigates from the linked agent surface without revoking in %s view', async view => {
    const user = userEvent.setup(); setup(); await screen.findByRole('link', { name: 'Beta' });
    if (view === 'list') await user.click(screen.getByRole('button', { name: 'List view' }));
    await user.click(screen.getByRole('link', { name: 'Beta' }));
    expect(screen.getByTestId('agents-path')).toHaveTextContent('/agents/beta');
    expect(screen.queryByRole('dialog', { name: 'Revoke access' })).not.toBeInTheDocument();
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
  });

  it('requires confirmation and reports success only when the server accepts revocation', async () => {
    let resolve!: () => void;
    vi.mocked(consentApi.deleteGrant).mockReturnValue(new Promise<void>((done) => { resolve = done; }));
    const user = userEvent.setup(); setup();
    await screen.findByText('Beta');
    await user.click(screen.getAllByRole('button', { name: 'Revoke', exact: true })[0]!);
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
    await user.click(within(screen.getByRole('dialog', { name: 'Revoke access' })).getByRole('button', { name: 'Revoke', exact: true }));
    await waitFor(() => expect(consentApi.deleteGrant).toHaveBeenCalledWith('alpha'));
    const pending = screen.getByText(rows[0]!.displayName).closest('[data-testid="agent-entity"]')!;
    expect(pending).toHaveAttribute('aria-busy', 'true');
    expect(screen.queryByText('Access revoked.')).not.toBeInTheDocument();
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([rows[1]!]);
    await act(async () => resolve());
    await waitFor(() => expect(screen.queryByText(rows[0]!.displayName)).not.toBeInTheDocument());
    expect(await screen.findByText('Access revoked.')).toBeVisible();
  });

  it('keeps access and announces failure when revocation fails', async () => {
    vi.mocked(consentApi.deleteGrant).mockRejectedValue(new Error('Unavailable'));
    const user = userEvent.setup(); setup(); await screen.findByText('Beta');
    await user.click(screen.getAllByRole('button', { name: 'Revoke', exact: true })[1]!);
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke', exact: true }));
    expect(await screen.findByText('Could not revoke access. Try again.')).toBeVisible();
    expect(screen.getByText('Beta')).toBeVisible();
    expect(screen.queryByText('Access revoked.')).not.toBeInTheDocument();
  });

  it('removes expired access without refetching while the page stays open', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([{ ...rows[1]!, expiresAt: new Date(Date.now() + 1000).toISOString() }]);
    setup();
    // Commit identity first, then deliver the newly mounted list query.
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(screen.getByText('Beta')).toBeVisible();
    await act(async () => { await vi.advanceTimersByTimeAsync(997); });
    expect(screen.getByText('Beta')).toBeVisible();
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(screen.queryByText('Beta')).not.toBeInTheDocument();
    expect(consentApi.getAgentDelegations).toHaveBeenCalledTimes(1);
  });

  it('shows loading instead of an empty state until agents load', async () => {
    const response = Promise.withResolvers<AgentDelegation[]>();
    vi.mocked(consentApi.getAgentDelegations).mockReturnValue(response.promise);
    setup();
    await screen.findByRole('status', { name: 'Loading…' });
    expect(screen.queryByTestId('delegations-empty-state')).not.toBeInTheDocument();
    await act(async () => response.resolve(rows));
    expect(await screen.findAllByTestId('agent-entity')).toHaveLength(2);
    expect(screen.queryByRole('status', { name: 'Loading…' })).not.toBeInTheDocument();
  });

  it('names access expiring within seven days and preserves its absolute date', async () => {
    const expiresAt = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString();
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([{ ...rows[1]!, expiresAt }]);
    setup();
    const expiry = await screen.findByTestId('agent-expiry');
    expect(expiry).toHaveTextContent('Expires soon');
    expect(within(expiry).getByText(/\d{4}/, { selector: 'time' })).toHaveAttribute('dateTime', expiresAt);
    expect(screen.queryByText('435')).not.toBeInTheDocument();
  });

  it('does not present a failed first read as an empty account and offers retry', async () => {
    vi.mocked(consentApi.getAgentDelegations).mockRejectedValueOnce(new Error('Unavailable'));
    const user = userEvent.setup(); setup();
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load agents.');
    expect(screen.queryByText('Review and revoke the access you have delegated to agents.')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Beta')).toBeVisible();
  });

  it('cancels revocation without changing access and keeps search-empty separate from account-empty', async () => {
    const user = userEvent.setup(); setup(); await screen.findByText('Beta');
    await user.click(screen.getAllByRole('button', { name: 'Revoke', exact: true })[1]!);
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
    expect(screen.getByText('Beta')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    await user.type(screen.getByRole('textbox', { name: 'Search agents' }), 'no matching agent');
    expect(screen.queryByText('Review and revoke the access you have delegated to agents.')).not.toBeInTheDocument();
    const noResults = screen.getAllByRole('status').find(status => within(status).queryByRole('button', { name: 'Clear search' }))!;
    await user.click(within(noResults).getByRole('button', { name: 'Clear search' }));
    expect(screen.getByRole('button', { name: 'Search agents', exact: true })).toBeVisible();
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(2));
  });

  it('restores linked search and sort without chips and persists list view', async () => {
    const user = userEvent.setup(); setup('/agents?q=Be&sort=expiring');
    expect(await screen.findByRole('link', { name: 'Beta' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Search agents: Be', exact: true })).toBeVisible();
    expect(screen.getAllByTestId('agent-entity')).toHaveLength(1);
    expect(screen.queryByLabelText('Active agent filters')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Sort' }));
    expect(screen.getByRole('menuitemradio', { name: 'Expiring soonest' })).toHaveAttribute('aria-checked', 'true');
    await user.click(screen.getByRole('menuitemradio', { name: 'Name', exact: true }));
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search agents: Be' })).toBeVisible();
    expect(screen.getByTestId('agents-query')).toHaveTextContent('q=Be');
    expect(screen.getByTestId('agents-query')).not.toHaveTextContent('sort=');
    await user.click(screen.getByRole('button', { name: 'List view' }));
    expect(localStorage.getItem('aib.collection-view.agents')).toBe('list');
    expect(screen.getByRole('button', { name: 'List view' })).toHaveAttribute('aria-pressed', 'true');
    await user.click(screen.getByRole('button', { name: 'Search agents: Be' }));
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveValue('Be');
    await user.click(screen.getByRole('button', { name: 'Clear search' }));
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(2));
    expect(screen.getByTestId('agents-query')).toBeEmptyDOMElement();
  });

  it('orders real agent links by restored expiry and by recently changed after a toolbar choice', async () => {
    const now = Date.now();
    const day = 24 * 60 * 60 * 1000;
    const soon = new Date(now + 2 * day).toISOString();
    const sortable: AgentDelegation[] = [
      { agentId: 'no-expiry', displayName: 'Borealis', activeGrantCount: 1, lastModifiedAt: '2026-09-25T00:00:00Z' },
      { agentId: 'far-expiry', displayName: 'Cascade', activeGrantCount: 1, lastModifiedAt: '2026-09-01T00:00:00Z', expiresAt: new Date(now + 90 * day).toISOString() },
      { agentId: 'tied-z', displayName: 'Zephyr', activeGrantCount: 1, lastModifiedAt: '2026-09-20T00:00:00Z', expiresAt: soon },
      { agentId: 'tied-a', displayName: 'Aspen', activeGrantCount: 1, lastModifiedAt: '2026-09-20T00:00:00Z', expiresAt: soon },
      { agentId: 'middle-expiry', displayName: 'Delta', activeGrantCount: 1, lastModifiedAt: '2026-09-21T00:00:00Z', expiresAt: new Date(now + 30 * day).toISOString() },
    ];
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue(sortable);
    const user = userEvent.setup();
    setup('/agents?sort=expiring');
    const agentLinks = () => within(screen.getByTestId('agents-collection')).getAllByRole('link', { name: /^(?:Aspen|Zephyr|Delta|Cascade|Borealis)$/ }).map(link => link.getAttribute('href'));
    await waitFor(() => expect(agentLinks()).toEqual([
      '/agents/tied-a', '/agents/tied-z', '/agents/middle-expiry', '/agents/far-expiry', '/agents/no-expiry',
    ]));
    expect(screen.getByTestId('agents-query')).toHaveTextContent(/^\?sort=expiring$/);

    await user.click(screen.getByRole('button', { name: 'Sort' }));
    await user.click(screen.getByRole('menuitemradio', { name: 'Recently changed' }));
    expect(agentLinks()).toEqual([
      '/agents/no-expiry', '/agents/middle-expiry', '/agents/tied-a', '/agents/tied-z', '/agents/far-expiry',
    ]);
    expect(screen.getByTestId('agents-query')).toHaveTextContent(/^\?sort=recent$/);
  });

  it('defaults to list above twelve agents, then honors global and per-page choices', async () => {
    const many = Array.from({ length: 13 }, (_, index) => ({ ...rows[0]!, agentId: `agent-${index}`, displayName: `Agent ${index}` }));
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue(many);
    const automatic = setup();
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(13));
    expect(screen.getByRole('button', { name: 'List view' })).toHaveAttribute('aria-pressed', 'true');
    automatic.unmount();
    localStorage.setItem('aib.collection-view-default', 'grid');
    const global = setup();
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(13));
    expect(screen.getByRole('button', { name: 'Grid view' })).toHaveAttribute('aria-pressed', 'true');
    global.unmount();
    localStorage.setItem('aib.collection-view.agents', 'list');
    setup();
    await waitFor(() => expect(screen.getAllByTestId('agent-entity')).toHaveLength(13));
    expect(screen.getByRole('button', { name: 'List view' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('uses an explicit global list choice below the automatic row threshold', async () => {
    localStorage.setItem('aib.collection-view-default', 'list');
    setup();
    await screen.findByRole('link', { name: 'Beta' });
    expect(screen.getByRole('button', { name: 'List view' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('explains empty agents with an illustrated icon, not a data table', async () => {
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]); setup();
    const empty = await screen.findByTestId('delegations-empty-state');
    expect(empty).toHaveTextContent('Review and revoke the access you have delegated to agents.');
    expect(empty.querySelector('svg')).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });
});
