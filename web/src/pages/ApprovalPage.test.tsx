import { type ReactNode } from 'react';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { approvalApi } from '@services/api/approvals';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import type { ApproveResponseData, DenyResponseData, ToolApprovalDetail } from '../types/approval';
import { ApprovalPage } from './ApprovalPage';

const approval: ToolApprovalDetail = {
  id: 'one', principal: 'alice', agent_id: 'agent', agent_display_name: 'Research assistant', tool_name: 'read_file',
  arguments: { path: '/report' }, tool_pattern: 'read_file', params_pattern: { path: '/report' },
  pattern_preview: 'read_file(path=/report)', status: 'pending', approval_url: '/approvals/one',
  created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z',
};
let client: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => <QueryProvider client={client}>
  <MemoryRouter initialEntries={['/approvals/one']}><Routes><Route path="/approvals/:id" element={children} /></Routes></MemoryRouter>
</QueryProvider>;

beforeEach(() => {
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(approvalApi, 'getApproval').mockResolvedValue(approval);
});
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks(); });

it.each(['approve', 'deny'] as const)('shows only the %s action as busy until its actual request settles', async action => {
  const approved = Promise.withResolvers<ApproveResponseData>();
  const denied = Promise.withResolvers<DenyResponseData>();
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(approved.promise);
  const deny = vi.spyOn(approvalApi, 'denyApproval').mockReturnValue(denied.promise);
  const user = userEvent.setup();
  render(<ApprovalPage />, { wrapper });
  const approveButton = await screen.findByRole('button', { name: 'Approve once' });
  const denyButton = screen.getByRole('button', { name: 'Deny', exact: true });
  const selected = action === 'approve' ? approveButton : denyButton;
  const other = action === 'approve' ? denyButton : approveButton;

  await user.click(selected);
  await waitFor(() => expect(selected).toHaveAttribute('aria-busy', 'true'));
  expect(selected).toBeDisabled();
  expect(other).toBeDisabled();
  expect(other).not.toHaveAttribute('aria-busy', 'true');
  expect(approve).toHaveBeenCalledTimes(action === 'approve' ? 1 : 0);
  expect(deny).toHaveBeenCalledTimes(action === 'deny' ? 1 : 0);
  expect(screen.queryByTestId('approval-outcome')).not.toBeInTheDocument();

  await act(async () => {
    if (action === 'approve') approved.resolve({ id: 'one', status: 'approved', persistence: 'once', approved_at: '2026-01-01T00:01:00Z' });
    else denied.resolve({ id: 'one', status: 'denied', denied_at: '2026-01-01T00:01:00Z' });
  });
  await waitFor(() => expect(screen.getByTestId('approval-outcome')).toBeVisible());
  expect(approve).toHaveBeenCalledTimes(action === 'approve' ? 1 : 0);
  expect(deny).toHaveBeenCalledTimes(action === 'deny' ? 1 : 0);
});
