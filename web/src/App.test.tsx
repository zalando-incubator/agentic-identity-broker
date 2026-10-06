import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { sessionsApi } from '@services/api/sessions';
import { approvalApi } from '@services/api/approvals';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import type { ToolApprovalDetail } from './types/approval';
import App from './App';

vi.mock('@services/api/consent', () => ({ consentApi: {
  getUserInfo: vi.fn(), getAgentDelegations: vi.fn(), getAgentDetail: vi.fn(),
  getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn(),
} }));
vi.mock('@services/api/sessions', () => ({ sessionsApi: { listSessions: vi.fn() } }));
vi.mock('@services/api/approvals', () => ({ approvalApi: {
  getApproval: vi.fn(), listPendingApprovals: vi.fn(), listPermanentApprovals: vi.fn(),
} }));

const agentId = '00000000-0000-4000-8000-000000000001';
const approvalId = '00000000-0000-4000-8000-000000000002';
const user = { principal: 'route-user@example.com', displayName: 'Route User' };
const approval: ToolApprovalDetail = {
  id: approvalId, principal: user.principal, agent_id: agentId, agent_display_name: 'Route agent',
  tool_name: 'read_calendar', arguments: {}, tool_pattern: 'read_calendar', params_pattern: {},
  pattern_preview: 'read_calendar()', status: 'approved', persistence: 'once',
  approval_url: `/approvals/${approvalId}`, created_at: '2026-09-27T12:00:00Z',
  approved_at: '2026-09-27T12:01:00Z', expires_at: '2099-01-01T00:00:00Z',
};
const clients: QueryClient[] = [];

function openRoute(path: string) {
  window.history.replaceState({}, '', path);
  const client = createQueryClient();
  clients.push(client);
  return render(<ThemeProvider><QueryProvider client={client}><App /></QueryProvider></ThemeProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  vi.mocked(consentApi.getUserInfo).mockResolvedValue(user);
  vi.mocked(consentApi.getAgentDelegations).mockResolvedValue([]);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue([]);
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue({
    agent: { agentId, displayName: 'Route agent', description: 'Review calendar access.', permission_sets: [], active_session_service_ids: [], service_requirements: [] },
    services: [],
  });
  vi.mocked(sessionsApi.listSessions).mockResolvedValue([]);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([]);
  vi.mocked(approvalApi.getApproval).mockResolvedValue(approval);
});

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  window.history.replaceState({}, '', '/');
});

describe('route context isolation', () => {
  it.each(['/agents', '/connections', '/approvals', `/agents/${agentId}`])('keeps console navigation available at %s', async (path) => {
    openRoute(path);
    const navigation = await screen.findByRole('navigation', { name: 'Main navigation' }, { timeout: 5000 });
    expect(within(navigation).getByRole('link', { name: 'Agents', exact: true })).toHaveAttribute('href', '/agents');
    expect(screen.queryByTestId('agent-origin-label')).not.toBeInTheDocument();
  });

  it.each([`/agents/${agentId}?session_token=invalid`, `/agents/${agentId}?session_token=`])('never turns an invalid decision context into a console at %s', async (path) => {
    vi.mocked(consentApi.getAgentDetail).mockRejectedValue({ statusCode: 400, error: 'invalid_token', message: 'Restart the authorization request.' });
    openRoute(path);
    const alert = await screen.findByRole('alert');
    expect(screen.getByRole('main')).toContainElement(alert);
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(consentApi.createOrUpdateGrant).not.toHaveBeenCalled();
  });

  it('keeps tool review in a branded decision frame without console navigation', async () => {
    openRoute(`/approvals/${approvalId}`);
    await screen.findByRole('main');
    expect(screen.getByRole('img', { name: 'Agentic Identity Broker' })).toBeInTheDocument();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('replaces the root history entry with the canonical agents route', async () => {
    const length = window.history.length;
    openRoute('/');
    await waitFor(() => expect(window.location.pathname).toBe('/agents'));
    expect(window.history.length).toBe(length);
  });

  it.each(['/delegations', '/sessions', '/settings'])('does not retain a legacy browser alias at %s', async (path) => {
    openRoute(path);
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeVisible();
    expect(window.location.pathname).toBe(path);
    expect(screen.getByRole('link', { name: 'Go to Agents' })).toHaveAttribute('href', '/agents');
  });

  it('recovers an unknown route through an internal console link', async () => {
    const keyboard = userEvent.setup();
    openRoute('/not-a-broker-route');
    const main = await screen.findByRole('main');
    await keyboard.click(within(main).getByRole('link'));
    await waitFor(() => expect(window.location.pathname).toBe('/agents'));
  });
});
