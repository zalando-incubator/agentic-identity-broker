import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import type { QueryClient } from '@tanstack/react-query';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { approvalApi } from '@services/api/approvals';
import { consentApi } from '@services/api/consent';
import { sessionsApi } from '@services/api/sessions';
import type { ToolApprovalDetail } from '../../types/approval';
import ApprovalsPage from '../../pages/ApprovalsPage';
import { ConnectionsPage } from '../../pages/ConnectionsPage';
import ConsoleLayout from './ConsoleLayout';

const request: ToolApprovalDetail = {
  id: 'request', principal: 'alice', agent_id: 'agent', agent_display_name: 'Reader',
  tool_name: 'read_document', arguments: {}, tool_pattern: 'read_document', params_pattern: {},
  pattern_preview: 'read_document()', status: 'pending', approval_url: '/approvals/request',
  created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z',
};
let client: QueryClient;
let mobile = false;

beforeEach(() => {
  mobile = false;
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.stubGlobal('matchMedia', vi.fn((query: string) => ({
    matches: mobile && query === '(width < 48rem)', media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(),
    removeEventListener: vi.fn(), dispatchEvent: vi.fn(() => true),
  })));
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValue([request]);
  vi.spyOn(approvalApi, 'listPermanentApprovals').mockResolvedValue([]);
});
afterEach(() => {
  cleanup();
  client.clear();
  localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it('keeps the queue and sidebar on one authoritative count through failure and arrival without moving focus', async () => {
  render(<ThemeProvider><QueryProvider client={client}><MemoryRouter initialEntries={['/approvals']}>
    <ConsoleLayout><ApprovalsPage /></ConsoleLayout>
  </MemoryRouter></QueryProvider></ThemeProvider>);
  await screen.findByText('read_document');
  expect(screen.getByTestId('pending-approval-count')).toHaveTextContent(/^1$/);
  const search = screen.getByRole('button', { name: 'Search', exact: true });
  search.focus();

  vi.mocked(approvalApi.listPendingApprovals).mockRejectedValueOnce({ statusCode: 503, message: 'Offline' });
  await act(async () => { await client.refetchQueries({ queryKey: queryKeys.pending('alice') }); });
  await waitFor(() => expect(screen.getByRole('link', { name: 'Approvals', exact: true })).toHaveAccessibleDescription(/out of date/));
  expect(screen.getByTestId('pending-approval-count')).toHaveTextContent(/^1$/);
  expect(within(screen.getByTestId('pending-approvals')).getByText('read_document')).toBeVisible();
  expect(search).toHaveFocus();

  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([request, { ...request, id: 'new', tool_name: 'write_document' }]);
  await act(async () => { await client.refetchQueries({ queryKey: queryKeys.pending('alice') }); });
  await waitFor(() => expect(screen.getByTestId('pending-approval-count')).toHaveTextContent(/^2$/));
  expect(within(screen.getByTestId('pending-approvals')).getByText('write_document')).toBeVisible();
  expect(screen.getByTestId('approval-announcement')).toHaveTextContent(/1 new/);
  expect(search).toHaveFocus();
  expect(approvalApi.listPendingApprovals).toHaveBeenCalledTimes(3);
});

it.each(['/approvals', '/sessions'])('closes mobile navigation after command selection from %s, including the current route', async (path) => {
  mobile = true;
  vi.spyOn(consentApi, 'getAgentDelegations').mockResolvedValue([]);
  vi.spyOn(sessionsApi, 'listSessions').mockResolvedValue([{
    id: 'connection', service_id: 'github', service_display_name: 'GitHub',
    token_type: 'Bearer', scope: ['repo'], initiated_at: '2026-01-01T00:00:00Z',
    is_expired: false, access_token_expired: false, has_refresh_token: true,
    dependent_agent_count: 0, is_encrypted: true,
  }]);
  const user = userEvent.setup();
  render(<ThemeProvider><QueryProvider client={client}><MemoryRouter initialEntries={[path]}>
    <ConsoleLayout><Routes>
      <Route path="/approvals" element={<ApprovalsPage />} />
      <Route path="/sessions" element={<ConnectionsPage />} />
    </Routes></ConsoleLayout>
  </MemoryRouter></QueryProvider></ThemeProvider>);
  await user.click(await screen.findByRole('button', { name: 'Open navigation' }));
  await user.click(screen.getByRole('button', { name: 'Search', exact: true }));
  const command = await screen.findByRole('dialog', { name: 'Command palette' });
  await user.type(within(command).getByRole('combobox'), 'GitHub');
  await screen.findByRole('option', { name: 'GitHub' });
  await user.keyboard('{Enter}');
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(screen.getByRole('heading', { name: 'Connections', exact: true })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Open navigation' })).toBeVisible();
});
