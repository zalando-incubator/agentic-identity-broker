import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
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
const rows: AgentDelegation[] = [{ agentId: 'alpha', displayName: '<img src=x onerror=alert(1)> Alpha', activeGrantCount: 4, lastModifiedAt: '2026-01-01T00:00:00Z' }, { agentId: 'beta', displayName: 'Beta', activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z', expiresAt: '2099-01-01T00:00:00Z' }];
const clients: QueryClient[] = [];
function setup() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } }); clients.push(client);
  return render(<ThemeProvider><QueryProvider client={client}><MemoryRouter><DelegationsPage /><Toaster label="Notifications" closeButtonLabel="Close" /></MemoryRouter></QueryProvider></ThemeProvider>);
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDelegations).mockResolvedValue(rows);
});
afterEach(() => { toast.dismiss(); vi.unstubAllGlobals(); vi.useRealTimers(); clients.splice(0).forEach((client) => client.clear()); });

describe('Agents console', () => {
  it('renders only agent, expiry, and actions; escapes names and supports case-insensitive search', async () => {
    const user = userEvent.setup(); const { container } = setup();
    const table = await screen.findByRole('table', { name: 'Agents' });
    expect(screen.getByRole('heading', { name: 'Agents', level: 1 })).toBeVisible();
    expect(screen.getByText('Review and revoke the access you have delegated to agents.')).toBeVisible();
    expect(within(table).getAllByRole('columnheader').map((node) => node.textContent)).toEqual(['Agent', 'Expiry', 'Actions']);
    expect(screen.getByText(rows[0]!.displayName)).toBeVisible();
    expect(container.querySelector('img[src="x"]')).toBeNull();
    expect(screen.getByText('Until revoked')).toHaveClass('font-mono');
    expect(screen.queryByRole('combobox', { name: 'Status' })).not.toBeInTheDocument();
    const view = screen.getAllByRole('link', { name: 'View' })[0]!;
    expect(view).toHaveAttribute('href', '/agents/alpha');
    expect(container.querySelector('[data-variant="primary"]')).toBeNull();
    await user.type(screen.getByRole('searchbox', { name: 'Search agents' }), 'bETA');
    expect(screen.queryByText(rows[0]!.displayName)).not.toBeInTheDocument();
    expect(screen.getByText('Beta')).toBeVisible();
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
    expect(screen.getByText(rows[0]!.displayName).closest('tr')).toHaveAttribute('aria-busy', 'true');
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

  it('does not present a failed first read as an empty account and offers retry', async () => {
    vi.mocked(consentApi.getAgentDelegations).mockRejectedValueOnce(new Error('Unavailable'));
    const user = userEvent.setup(); setup();
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load agents.');
    expect(screen.queryByText('A delegation lets an agent access services on your behalf.')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Beta')).toBeVisible();
  });

  it('cancels revocation without changing access and keeps search-empty separate from account-empty', async () => {
    const user = userEvent.setup(); setup(); await screen.findByText('Beta');
    await user.click(screen.getAllByRole('button', { name: 'Revoke', exact: true })[1]!);
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(consentApi.deleteGrant).not.toHaveBeenCalled();
    expect(screen.getByText('Beta')).toBeVisible();
    await user.type(screen.getByRole('searchbox', { name: 'Search agents' }), 'no matching agent');
    expect(screen.getByText('No agents match your search.')).toBeVisible();
    expect(screen.queryByText('A delegation lets an agent access services on your behalf.')).not.toBeInTheDocument();
  });

  it('explains empty delegations with local branding', async () => {
    vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]); setup();
    expect(await screen.findByText('A delegation lets an agent access services on your behalf.')).toBeVisible();
    expect(screen.getByRole('img', { name: 'Agentic Identity Broker' })).toBeVisible();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });
});
