import type { ReactNode } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { approvalApi } from '@services/api/approvals';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { PendingApprovalsProvider } from '@hooks/usePendingApprovals';
import { ApprovalArrivalAnnouncer } from '@components/approvals/ApprovalArrivalAnnouncer';
import type { ToolApprovalDetail } from '../types/approval';
import ApprovalsPage from './ApprovalsPage';

const pending: ToolApprovalDetail = { id: 'pending', principal: 'alice', agent_id: 'agent', agent_display_name: 'Reader agent', tool_name: 'read_document', risk_level: 'medium', arguments: { path: '/docs/a' }, tool_pattern: 'read_document', params_pattern: { path: '/docs/a' }, pattern_preview: 'read_document(path=/docs/a)', status: 'pending', approval_url: '/approvals/pending', created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z' };
let client: QueryClient;
function Wrapper({ children }: { children: ReactNode }) {
  return <QueryProvider client={client}><PendingApprovalsProvider><div aria-live="polite" data-testid="approval-announcement"><ApprovalArrivalAnnouncer /></div>{children}</PendingApprovalsProvider></QueryProvider>;
}
beforeEach(() => {
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValue([pending]);
  vi.spyOn(approvalApi, 'listPermanentApprovals').mockResolvedValue([]);
  vi.spyOn(approvalApi, 'previewApprovalScope').mockResolvedValue({ tool_pattern: 'read_document', params_pattern: { path: '/docs/a' }, preview: 'read_document(path=/docs/a)' });
});
afterEach(() => { cleanup(); client.clear(); vi.useRealTimers(); vi.restoreAllMocks(); });

it('orders pending before standing allow and deny, with named non-accent row decisions', async () => {
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('read_document');
  const headings = screen.getAllByRole('heading').map(heading => heading.textContent);
  expect(headings.indexOf('Pending requests')).toBeLessThan(headings.indexOf('Standing allow decisions'));
  expect(headings.indexOf('Standing allow decisions')).toBeLessThan(headings.indexOf('Standing deny decisions'));
  const row = screen.getByRole('row', { name: /read_document/ });
  expect(within(row).getByText('Reader agent')).toBeVisible();
  expect(within(row).getByText(/Medium risk/i)).toBeVisible();
  expect(within(row).getByRole('time')).toHaveAttribute('datetime', pending.created_at);
  expect(within(row).getByRole('button', { name: 'Approve', exact: true })).toHaveAttribute('data-variant', 'secondary');
  expect(within(row).getByRole('button', { name: 'Deny', exact: true })).toHaveAttribute('data-variant', 'outline');
});

it.each(['This session', 'Always'])('previews %s without submitting and keeps the row until explicit approval succeeds', async label => {
  const response = Promise.withResolvers<{ id: string; status: 'approved'; persistence: 'session' | 'permanent'; approved_at: string }>();
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(response.promise);
  render(<ApprovalsPage />, { wrapper: Wrapper });
  fireEvent.click(await screen.findByRole('button', { name: 'Approve', exact: true }));
  fireEvent.click(screen.getByRole('radio', { name: new RegExp(label, 'i') }));
  await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalled());
  const confirm = screen.getByRole('button', { name: 'Confirm approve' });
  await waitFor(() => expect(confirm).toBeEnabled());
  expect(approve).not.toHaveBeenCalled();
  fireEvent.click(confirm);
  await waitFor(() => expect(approve).toHaveBeenCalledWith('pending', { persistence: label === 'Always' ? 'permanent' : 'session', params_pattern: { path: '/docs/a' } }));
  expect(screen.getByText('read_document')).toBeVisible();
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  await act(async () => response.resolve({ id: 'pending', status: 'approved', persistence: label === 'Always' ? 'permanent' : 'session', approved_at: '2026-09-01T00:00:00Z' }));
  await screen.findByText('No pending approvals');
});

it('does not deny on opening review and refreshes only after the confirmed server outcome', async () => {
  const response = Promise.withResolvers<{ id: string; status: 'denied'; denied_at: string }>();
  const deny = vi.spyOn(approvalApi, 'denyApproval').mockReturnValue(response.promise);
  render(<ApprovalsPage />, { wrapper: Wrapper });
  fireEvent.click(await screen.findByRole('button', { name: 'Deny', exact: true }));
  expect(deny).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Deny this request' }));
  await waitFor(() => expect(deny).toHaveBeenCalledWith('pending', expect.anything()));
  expect(deny.mock.calls[0][1]?.persistence).toBeUndefined();
  expect(screen.getByText('read_document')).toBeVisible();
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  await act(async () => response.resolve({ id: 'pending', status: 'denied', denied_at: '2026-09-01T00:00:00Z' }));
  await screen.findByText('No pending approvals');
});

it.each([
  ['Approve', 'Confirm approve'],
  ['Deny', 'Deny this request'],
] as const)('expires a pending row with an open %s confirmation at its deadline without a refresh', async (action, confirmation) => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  vi.setSystemTime(new Date('2026-10-03T12:00:00Z'));
  const expiresAt = Date.now() + 4_000;
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([{ ...pending, expires_at: new Date(expiresAt).toISOString() }]);
  const approve = vi.spyOn(approvalApi, 'approveApproval');
  const deny = vi.spyOn(approvalApi, 'denyApproval');
  render(<ApprovalsPage />, { wrapper: Wrapper });
  // Commit the identity boundary, then deliver the pending query.
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  const row = screen.getByRole('row', { name: /read_document/ });
  const approveButton = within(row).getByRole('button', { name: 'Approve', exact: true });
  const denyButton = within(row).getByRole('button', { name: 'Deny', exact: true });
  fireEvent.click(action === 'Approve' ? approveButton : denyButton);
  const confirmButton = within(row).getByRole('button', { name: confirmation });
  expect(confirmButton).toBeEnabled();

  await act(async () => { await vi.advanceTimersByTimeAsync(expiresAt - Date.now() - 1); });
  expect(confirmButton).toBeEnabled();
  expect(within(row).queryByRole('status')).not.toBeInTheDocument();
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  expect(within(row).getByRole('status')).toHaveTextContent('This request has expired. No decision can be submitted.');
  expect(approveButton).toBeDisabled();
  expect(denyButton).toBeDisabled();
  expect(confirmButton).toBeDisabled();
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
  expect(approvalApi.listPendingApprovals).toHaveBeenCalledTimes(1);
});

it('revokes a standing deny only after confirmation', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'denied', status: 'denied', persistence: 'permanent' }]);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  const revoke = vi.spyOn(approvalApi, 'revokePermanentApproval').mockResolvedValue({ id: 'denied', status: 'denied', denied_at: '2026-09-01T00:00:00Z' });
  render(<ApprovalsPage />, { wrapper: Wrapper });
  fireEvent.click(await screen.findByRole('button', { name: 'Revoke', exact: true }));
  expect(revoke).not.toHaveBeenCalled();
  const dialog = await screen.findByRole('dialog', { name: 'Revoke standing decision?' });
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([]);
  fireEvent.click(within(dialog).getByRole('button', { name: 'Revoke decision' }));
  await waitFor(() => expect(revoke).toHaveBeenCalledWith('denied'));
  await screen.findByText('No standing deny decisions');
});

it('bounds unbroken tool and agent names in revoke confirmation and expands escaped text by keyboard', async () => {
  const user = userEvent.setup();
  const toolName = `${'t'.repeat(320)}<img src=x onerror=alert(1)>`;
  const agentName = `${'a'.repeat(320)}<script>alert(2)</script>`;
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{
    ...pending, id: 'denied', status: 'denied', persistence: 'permanent',
    tool_name: toolName, agent_display_name: agentName,
  }]);
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await user.click(await screen.findByRole('button', { name: 'Revoke', exact: true }));
  const dialog = await screen.findByRole('dialog', { name: 'Revoke standing decision?' });
  const expand = within(dialog).getByRole('button', { name: 'Show more' });
  const context = document.getElementById(expand.getAttribute('aria-controls')!);
  expect(context).toHaveClass('line-clamp-2');
  expect(context).toHaveTextContent(`${toolName} for ${agentName}`);
  expect(dialog.querySelector('img, script')).toBeNull();
  expect(within(dialog).getByText('Future matching requests will need a new decision.')).toBeVisible();

  for (let i = 0; i < 5 && document.activeElement !== expand; i++) await user.tab();
  expect(expand).toHaveFocus();
  await user.keyboard('{Enter}');
  expect(within(dialog).getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true');
  expect(context).not.toHaveClass('line-clamp-2');
  expect(within(dialog).getByText('Future matching requests will need a new decision.')).toBeVisible();
  await user.keyboard(' ');
  expect(expand).toHaveAttribute('aria-expanded', 'false');
  expect(context).toHaveClass('line-clamp-2');
});

it('announces arrivals and completed decisions without moving focus or adding a live region', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'allow', tool_name: 'standing_tool', status: 'approved', persistence: 'permanent' }]);
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('read_document');
  const focused = await screen.findByRole('button', { name: 'Revoke', exact: true });
  focused.focus();
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, { ...pending, id: 'new', tool_name: 'write_document' }]);
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.pending('alice') }); });
  await screen.findByText('write_document');
  expect(screen.getByTestId('approval-announcement')).toHaveTextContent('1 new approval request');
  expect(document.activeElement).toBe(focused);
  expect(document.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending]);
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.pending('alice') }); });
  await waitFor(() => expect(screen.getByTestId('approval-announcement')).toHaveTextContent('1 approval request updated'));
  expect(document.activeElement).toBe(focused);
});

it('keeps stale rows on refresh failure and does not turn an initial failure into empty success', async () => {
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('read_document');
  vi.mocked(approvalApi.listPendingApprovals).mockRejectedValue(new Error('offline'));
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.pending('alice') }); });
  expect(screen.getByText('read_document')).toBeVisible();
  expect(await screen.findByText('Pending approvals may be out of date.')).toBeVisible();
  cleanup(); client.clear();
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('Pending approvals could not be loaded.');
  expect(screen.queryByText('No pending approvals')).not.toBeInTheDocument();
});

it('keeps confirmation disabled when a remembered scope cannot be previewed', async () => {
  vi.mocked(approvalApi.previewApprovalScope).mockRejectedValue({ status: 422, code: 'invalid_pattern', message: 'Invalid pattern' });
  const approve = vi.spyOn(approvalApi, 'approveApproval');
  render(<ApprovalsPage />, { wrapper: Wrapper });
  fireEvent.click(await screen.findByRole('button', { name: 'Approve', exact: true }));
  fireEvent.click(screen.getByRole('radio', { name: /This session/i }));
  await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalled());
  expect(screen.getByRole('button', { name: 'Confirm approve' })).toBeDisabled();
  expect(approve).not.toHaveBeenCalled();
});

it('shows loading without inventing an empty list, then displays both truthful empty states', async () => {
  const response = Promise.withResolvers<ToolApprovalDetail[]>();
  vi.mocked(approvalApi.listPendingApprovals).mockReturnValue(response.promise);
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('Loading pending approvals.');
  expect(screen.queryByText('No pending approvals')).not.toBeInTheDocument();
  await act(async () => response.resolve([]));
  await screen.findByText('No pending approvals');
  expect(screen.getByText('No standing allow decisions')).toBeVisible();
  expect(screen.getByText('No standing deny decisions')).toBeVisible();
});

it('retains standing decisions on a failed refresh without reporting an empty section', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'allow', tool_name: 'standing_tool', status: 'approved', persistence: 'permanent' }]);
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('standing_tool');
  vi.mocked(approvalApi.listPermanentApprovals).mockRejectedValue(new Error('offline'));
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.standing('alice') }); });
  expect(screen.getByText('standing_tool')).toBeVisible();
  expect(await screen.findByText('Standing decisions may be out of date.')).toBeVisible();
  expect(screen.queryByText('No standing allow decisions')).not.toBeInTheDocument();
});

it('does not show previous-principal queue rows or announcements after identity changes', async () => {
  render(<ApprovalsPage />, { wrapper: Wrapper });
  await screen.findByText('read_document');
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  await act(async () => { await client.invalidateQueries({ queryKey: ['identity'] }); });
  await screen.findByText('No pending approvals');
  expect(screen.queryByText('read_document')).not.toBeInTheDocument();
  expect(screen.getByTestId('approval-announcement')).toBeEmptyDOMElement();
});
