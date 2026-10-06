import { useState } from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { sessionsApi } from '@services/api/sessions';
import { approvalApi } from '@services/api/approvals';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { ThemeProvider, useTheme } from '@design-system/theme/ThemeProvider';
import { PendingApprovalsProvider } from '@hooks/usePendingApprovals';
import { useCommandShortcut } from '@hooks/useCommandShortcut';
import { CommandPalette } from './CommandPalette';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDelegations: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn() } }));
vi.mock('@services/api/sessions', () => ({ sessionsApi: { listSessions: vi.fn(), getSessionDetails: vi.fn() } }));
vi.mock('@services/api/approvals', () => ({ approvalApi: { listPendingApprovals: vi.fn(), listPermanentApprovals: vi.fn(), getApproval: vi.fn() } }));
const clients: QueryClient[] = [];
const unsafeName = '<img src=x onerror=alert(1)> Agent';
function ConsoleHarness() {
  const [open, setOpen] = useState(false);
  const location = useLocation();
  const { preference } = useTheme();
  useCommandShortcut(() => setOpen((value) => !value));
  return <><button onClick={() => setOpen(true)}>Search</button><output aria-label="Current route">{location.pathname}</output><output aria-label="Theme preference">{preference}</output><CommandPalette open={open} onOpenChange={setOpen} /></>;
}
function setup() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } }); clients.push(client);
  client.setQueryData(queryKeys.delegations('bob'), [{ agentId: 'private', displayName: 'Private Bob agent' }]);
  return render(<ThemeProvider><QueryProvider client={client}><PendingApprovalsProvider><MemoryRouter initialEntries={['/agents']}><ConsoleHarness /></MemoryRouter></PendingApprovalsProvider></QueryProvider></ThemeProvider>);
}
beforeEach(() => {
  vi.resetAllMocks(); localStorage.clear();
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([{ agentId: 'agent', displayName: unsafeName, activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z' }]);
  vi.mocked(sessionsApi.listSessions).mockResolvedValue([{ id: 'session', service_id: 'github', service_display_name: 'GitHub', token_type: 'Bearer', scope: ['read'], initiated_at: '2026-01-01T00:00:00Z', is_expired: false, access_token_expired: false, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true }]);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([{ id: 'approval', principal: 'alice', agent_id: 'agent', tool_name: 'read_file', arguments: {}, tool_pattern: 'read_file', params_pattern: {}, pattern_preview: 'read_file()', status: 'pending', approval_url: '/approvals/approval', created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z' }]);
});
afterEach(() => { vi.unstubAllGlobals(); clients.splice(0).forEach((client) => client.clear()); });

describe('console command palette', () => {
  it.each(['control', 'meta', 'button'])('opens with %s and traps focus until Escape returns to the opener', async (trigger) => {
    const user = userEvent.setup(); setup();
    const search = await screen.findByRole('button', { name: 'Search', exact: true }); search.focus();
    if (trigger === 'button') await user.click(search);
    else fireEvent.keyDown(document, { key: 'k', ctrlKey: trigger === 'control', metaKey: trigger === 'meta' });
    const dialog = await screen.findByRole('dialog', { name: 'Command palette' });
    expect(within(dialog).getByRole('combobox')).toHaveFocus();
    await user.tab(); expect(dialog.contains(document.activeElement)).toBe(true);
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(search).toHaveFocus();
  });

  it('groups acting-user records and never reads detail or other-principal data', async () => {
    const user = userEvent.setup(); const { container } = setup();
    await user.click(await screen.findByRole('button', { name: 'Search', exact: true }));
    expect(await screen.findByRole('option', { name: unsafeName })).toBeVisible();
    for (const group of ['Agents', 'Connections', 'Approvals', 'Theme']) expect(screen.getByText(group)).toBeVisible();
    expect(screen.queryByText('Private Bob agent')).not.toBeInTheDocument();
    expect(container.querySelector('img[src="x"]')).toBeNull();
    expect(consentApi.getAgentDetail).not.toHaveBeenCalled(); expect(consentApi.getAgentGrants).not.toHaveBeenCalled();
    expect(sessionsApi.getSessionDetails).not.toHaveBeenCalled(); expect(approvalApi.getApproval).not.toHaveBeenCalled(); expect(approvalApi.listPermanentApprovals).not.toHaveBeenCalled();
  });

  it.each([[unsafeName, '/agents/agent'], ['GitHub', '/connections'], ['read_file', '/approvals/approval']])('searches and navigates %s', async (name, route) => {
    const user = userEvent.setup(); setup();
    await user.click(await screen.findByRole('button', { name: 'Search', exact: true }));
    await screen.findByRole('option', { name, exact: true });
    await user.type(screen.getByRole('combobox'), name === unsafeName ? 'Agent' : name);
    await user.keyboard('{Enter}');
    expect(screen.getByLabelText('Current route')).toHaveTextContent(route);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('applies theme without navigating or changing authorization data', async () => {
    const user = userEvent.setup(); setup();
    await user.click(await screen.findByRole('button', { name: 'Search', exact: true }));
    await user.type(screen.getByRole('combobox'), 'Dark');
    await user.keyboard('{Enter}');
    expect(screen.getByLabelText('Theme preference')).toHaveTextContent('dark');
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark');
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/agents');
  });
});
