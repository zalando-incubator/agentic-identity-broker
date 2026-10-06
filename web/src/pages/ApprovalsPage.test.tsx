import type { ReactNode } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { QueryClient } from '@tanstack/react-query';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { approvalApi } from '@services/api/approvals';
import { consentApi } from '@services/api/consent';
import { QueryProvider } from '@services/query/QueryProvider';
import { createQueryClient } from '@services/query/queryClient';
import { queryKeys } from '@services/query/queryKeys';
import { PendingApprovalsProvider } from '@hooks/usePendingApprovals';
import { ApprovalArrivalAnnouncer } from '@components/approvals/ApprovalArrivalAnnouncer';
import type { ToolApprovalDetail } from '../types/approval';
import { collectionCopy, navigationCopy } from '@copy';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';
import ApprovalsPage from './ApprovalsPage';

const pending: ToolApprovalDetail = { id: 'pending', principal: 'alice', agent_id: 'agent', agent_display_name: 'Reader agent', tool_name: 'read_document', risk_level: 'medium', arguments: { path: '/docs/a' }, tool_pattern: 'read_document', params_pattern: { path: '/docs/a' }, pattern_preview: 'read_document(path=/docs/a)', status: 'pending', approval_url: '/approvals/pending', created_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z' };
const second: ToolApprovalDetail = { ...pending, id: 'next', tool_name: 'write_document', approval_url: '/approvals/next' };
let client: QueryClient;
function LocationProbe() {
  const location = useLocation();
  return <output data-testid="approval-location">{`${location.pathname}${location.search}`}</output>;
}
function renderPage(route = '/approvals') {
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryProvider client={client}><PendingApprovalsProvider><MemoryRouter initialEntries={[route]}>
      <div aria-live="polite" data-testid="approval-announcement"><ApprovalArrivalAnnouncer /></div>
      <LocationProbe />{children}
    </MemoryRouter></PendingApprovalsProvider></QueryProvider>;
  }
  return render(<ApprovalsPage />, { wrapper: Wrapper });
}
function panel() { return screen.getByRole('region', { name: /^Selected request:/ }); }

beforeEach(() => {
  client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false }, mutations: { retry: false } });
  vi.spyOn(consentApi, 'getUserInfo').mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.spyOn(approvalApi, 'listPendingApprovals').mockResolvedValue([pending]);
  vi.spyOn(approvalApi, 'listPermanentApprovals').mockResolvedValue([]);
  vi.spyOn(approvalApi, 'previewApprovalScope').mockResolvedValue({ tool_pattern: 'read_document', params_pattern: { path: '/docs/a' }, preview: pending.pattern_preview });
});
afterEach(() => { cleanup(); client.clear(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

it('shows a stable pending EntityRow list beside the shared detail panel and route tabs', async () => {
  renderPage();
  const list = await screen.findByTestId('pending-approvals');
  const row = within(list).getByTestId('pending-approval-row');
  expect(row).toHaveAttribute('data-slot', 'entity-row');
  expect(row).toHaveAttribute('data-selected', 'true');
  expect(row).toHaveAttribute('data-layout', 'stacked');
  expect(within(row).getByRole('button', { name: pending.tool_name })).toHaveAttribute('aria-controls', `approval-review-${pending.id}`);
  expect(document.getElementById(`approval-review-${pending.id}`)).toContainElement(panel());
  expect(panel()).toHaveAttribute('aria-label', `Selected request: ${pending.tool_name} for Reader agent`);
  expect(within(panel()).getByTestId('selected-approval-identity')).toHaveTextContent(pending.tool_name);
  expect(within(panel()).getByTestId('selected-approval-identity')).toHaveTextContent('Reader agent');
  expect(within(row).getByText('Reader agent')).toBeVisible();
  expect(within(row).getByRole('time')).toHaveAttribute('datetime', pending.created_at);
  expect(within(panel()).getByText(pending.pattern_preview)).toBeVisible();
  expect(within(panel()).getByText('/docs/a')).toBeVisible();
  expect(within(panel()).getByRole('button', { name: 'Deny', exact: true })).toHaveAttribute('data-variant', 'outline');
  expect(within(panel()).getByRole('button', { name: 'Approve once' })).toHaveAttribute('data-variant', 'primary');
  expect(screen.getByRole('tab', { name: /Pending/ })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByRole('tab', { name: /^Remembered/ })).toHaveAttribute('href', '/approvals/remembered');
});

it('shows the principal-scoped remembered total before visiting the tab and keeps it independent of filters', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([
    { ...pending, id: 'allow', status: 'approved', persistence: 'permanent' },
    { ...pending, id: 'deny', status: 'denied', persistence: 'permanent' },
    { ...pending, id: 'foreign', principal: 'bob', status: 'approved', persistence: 'permanent' },
  ]);
  const user = userEvent.setup();
  renderPage();
  const tab = await screen.findByRole('tab', { name: 'Remembered · 2' });
  await user.click(tab);
  await user.click(screen.getByRole('button', { name: `${copy.filter}: ${copy.all}` }));
  await user.click(within(await screen.findByRole('group', { name: copy.filter })).getByRole('button', { name: copy.denied }));
  await waitFor(() => expect(screen.getAllByTestId('standing-decision-row')).toHaveLength(1));
  expect(tab).toHaveAccessibleName('Remembered · 2');
  await act(async () => { client.setQueryData(queryKeys.standing('alice'), []); });
  await waitFor(() => expect(tab).toHaveAccessibleName('Remembered · 0'));
});

it('honors the displayed decision shortcut when focus is on its main button', async () => {
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: pending.id, status: 'approved', persistence: 'once', approved_at: '2026-10-06T10:00:00Z' });
  const user = userEvent.setup();
  renderPage();
  const action = await screen.findByRole('button', { name: 'Approve once' });
  action.focus();
  await user.keyboard('a');
  await waitFor(() => expect(approve).toHaveBeenCalledWith(pending.id, { persistence: 'once' }));
});

it('keeps the header and tabs mounted while remembered controls stay in their panel and pending decisions disappear immediately', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'allowed', status: 'approved', persistence: 'permanent' }]);
  const approve = vi.spyOn(approvalApi, 'approveApproval');
  const deny = vi.spyOn(approvalApi, 'denyApproval');
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('pending-approvals');
  const heading = screen.getByRole('heading', { level: 1, name: navigationCopy.approvals });
  const tabs = screen.getByRole('tablist', { name: navigationCopy.approvals });
  expect(screen.queryByRole('button', { name: copy.searchRemembered })).not.toBeInTheDocument();

  await user.click(screen.getByRole('tab', { name: /^Remembered/ }));
  const remembered = screen.getByRole('tabpanel', { name: /^Remembered/ });
  expect(screen.getByRole('heading', { level: 1, name: navigationCopy.approvals })).toBe(heading);
  expect(screen.getByRole('tablist', { name: navigationCopy.approvals })).toBe(tabs);
  expect(within(remembered).getByRole('button', { name: `${copy.filter}: ${copy.all}` })).toBeInTheDocument();
  expect(within(remembered).getByRole('button', { name: copy.searchRemembered })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Approve once' })).not.toBeInTheDocument();
  fireEvent.keyDown(document, { key: 'a' });
  fireEvent.keyDown(document, { key: 'd' });
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
  await user.click(within(remembered).getByRole('button', { name: copy.searchRemembered }));
  const search = within(remembered).getByRole('textbox', { name: copy.searchRemembered });
  expect(search).toHaveFocus();
  await user.type(search, 'read_document');
  expect(screen.getByTestId('approval-location')).toHaveTextContent('/approvals/remembered?q=read_document');

  await user.click(screen.getByRole('tab', { name: /Pending/ }));
  expect(screen.getByRole('heading', { level: 1, name: navigationCopy.approvals })).toBe(heading);
  expect(screen.getByRole('tablist', { name: navigationCopy.approvals })).toBe(tabs);
  expect(screen.queryByRole('textbox', { name: copy.searchRemembered })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: `${copy.filter}: ${copy.all}` })).not.toBeInTheDocument();
  expect(screen.getByTestId('pending-approvals')).toBeInTheDocument();
  expect(within(panel()).getByRole('button', { name: 'Approve once' })).toBeEnabled();
});

it('keeps the selected tool and agent tied to the fixed list and decides the keyboard-selected request', async () => {
  const writer = { ...second, agent_id: 'writer', agent_display_name: 'Writer agent' };
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, writer]);
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: writer.id, status: 'approved', persistence: 'once', approved_at: '2026-10-05T10:00:00Z' });
  const user = userEvent.setup();
  renderPage();
  const list = await screen.findByTestId('pending-approvals');
  await waitFor(() => expect(list).toHaveStyle({ minHeight: '188px' }));
  const rows = within(list).getAllByTestId('pending-approval-row');
  expect(rows[0]).toHaveAttribute('data-selected', 'true');
  await user.click(within(rows[1]).getByRole('button', { name: writer.tool_name }));
  expect(rows[0]).not.toHaveAttribute('data-selected');
  expect(rows[1]).toHaveAttribute('data-selected', 'true');
  expect(within(rows[1]).getByRole('button', { name: writer.tool_name })).toHaveAttribute('aria-controls', `approval-review-${writer.id}`);
  expect(within(panel()).getByRole('heading', { name: writer.tool_name, exact: true })).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: 'Approve once' })).toHaveLength(1);
  const exiting = document.getElementById(`approval-review-${pending.id}`);
  if (exiting) {
    expect(exiting).toHaveAttribute('inert');
    expect(exiting).toHaveAttribute('aria-hidden', 'true');
  }
  expect(within(panel()).getByTestId('selected-approval-identity')).toHaveTextContent(writer.agent_display_name);
  await user.keyboard('k');
  expect(within(panel()).getByTestId('selected-approval-identity')).toHaveTextContent('Reader agent');
  await user.keyboard('j');
  expect(within(panel()).getByTestId('selected-approval-identity')).toHaveTextContent(writer.agent_display_name);
  expect(list).toHaveStyle({ minHeight: '188px' });
  await user.click(within(panel()).getByRole('button', { name: 'Approve once' }));
  await waitFor(() => expect(approve).toHaveBeenCalledWith(writer.id, { persistence: 'once' }));
});

it('opens narrow-screen requests as links and never decides through hidden-panel shortcuts', async () => {
  const media = Object.assign(new EventTarget(), { matches: true });
  vi.stubGlobal('matchMedia', (query: string) => query === '(max-width: 1011px)' ? media : Object.assign(new EventTarget(), { matches: false }));
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: pending.id, status: 'approved', persistence: 'once', approved_at: '2026-10-05T10:00:00Z' });
  const deny = vi.spyOn(approvalApi, 'denyApproval');
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  renderPage();
  const list = await screen.findByTestId('pending-approvals');
  expect(within(list).getByRole('link', { name: pending.tool_name })).toHaveAttribute('href', `/approvals/${pending.id}`);
  expect(within(list).getByRole('link', { name: second.tool_name })).toHaveAttribute('href', `/approvals/${second.id}`);
  expect(within(list).queryByRole('button', { name: pending.tool_name })).not.toBeInTheDocument();
  await act(async () => {
    for (const key of ['a', 'd', 'j', 'k']) fireEvent.keyDown(document, { key });
  });
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
  act(() => { media.matches = false; media.dispatchEvent(new Event('change')); });
  expect(within(list).getByRole('button', { name: pending.tool_name })).toBeVisible();
  const rowControl = within(list).getByRole('button', { name: pending.tool_name });
  rowControl.focus();
  fireEvent.keyDown(rowControl, { key: 'a' });
  await waitFor(() => expect(approve).toHaveBeenCalledWith(pending.id, { persistence: 'once' }));
});

it.each(['middle', 'last'] as const)('selects the adjacent request after the %s request is accepted', async position => {
  const third = { ...pending, id: 'third', tool_name: 'send_document', approval_url: '/approvals/third' };
  const decided = position === 'middle' ? second : third;
  const next = position === 'middle' ? third : second;
  const remaining = [pending, second, third].filter(request => request.id !== decided.id);
  const response = Promise.withResolvers<{ id: string; status: 'approved'; persistence: 'once'; approved_at: string }>();
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(response.promise);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second, third]);
  const user = userEvent.setup();
  renderPage();
  const list = await screen.findByTestId('pending-approvals');
  await user.click(within(list).getByRole('button', { name: decided.tool_name }));
  await user.click(within(panel()).getByRole('button', { name: 'Approve once' }));
  await waitFor(() => expect(approve).toHaveBeenCalledWith(decided.id, { persistence: 'once' }));
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue(remaining);
  await act(async () => response.resolve({ id: decided.id, status: 'approved', persistence: 'once', approved_at: '2026-10-05T10:00:00Z' }));
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: next.tool_name })).toBeInTheDocument());
  expect(within(list).getByRole('button', { name: next.tool_name }).closest('[data-testid="pending-approval-row"]')).toHaveAttribute('data-selected', 'true');
});

it('keeps a newly selected request after another decision settles and blocks decisions while it is pending', async () => {
  const third = { ...pending, id: 'third', tool_name: 'delete_document', approval_url: '/approvals/third' };
  const response = Promise.withResolvers<{ id: string; status: 'approved'; persistence: 'once'; approved_at: string }>();
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(response.promise);
  const deny = vi.spyOn(approvalApi, 'denyApproval');
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second, third]);
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  await user.click(screen.getByRole('button', { name: pending.tool_name, exact: true }));
  await user.click(within(panel()).getByRole('button', { name: 'Approve once' }));
  await waitFor(() => expect(approve).toHaveBeenCalledWith(pending.id, { persistence: 'once' }));
  await user.click(screen.getByRole('button', { name: third.tool_name, exact: true }));
  expect(within(panel()).getByRole('heading', { name: third.tool_name, exact: true })).toBeInTheDocument();
  expect(within(panel()).getByRole('button', { name: 'Approve once' })).toBeDisabled();
  await user.keyboard('adjk');
  expect(approve).toHaveBeenCalledTimes(1);
  expect(deny).not.toHaveBeenCalled();
  expect(within(panel()).getByRole('heading', { name: third.tool_name, exact: true })).toBeInTheDocument();
  for (const name of ['Deny', 'Approve once']) {
    const action = within(panel()).getByRole('button', { name, exact: true });
    expect(action).toHaveAttribute('data-size', 'default');
    expect(within(action).getByText(name)).toHaveClass('whitespace-normal');
    expect(within(action).getByText(name)).not.toHaveClass('sr-only');
  }
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([second, third]);
  await act(async () => { response.resolve({ id: pending.id, status: 'approved', persistence: 'once', approved_at: '2026-10-04T10:00:00Z' }); });
  await waitFor(() => expect(within(panel()).getByRole('button', { name: 'Approve once' })).toBeEnabled());
  expect(screen.getByRole('button', { name: third.tool_name, exact: true }).closest('[data-testid="pending-approval-row"]')).toHaveAttribute('data-selected', 'true');
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: third.tool_name, exact: true })).toBeInTheDocument());
  expect(approve).toHaveBeenCalledTimes(1);
});

it.each([['For this session', 'session'], ['Always…', 'permanent']] as const)('keeps list geometry fixed in the %s scope editor and chooses the next request only after server acceptance', async (label, persistence) => {
  const response = Promise.withResolvers<{ id: string; status: 'approved'; persistence: 'session' | 'permanent'; approved_at: string }>();
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockReturnValue(response.promise);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  const user = userEvent.setup();
  renderPage();
  const list = await screen.findByTestId('pending-approvals');
  await screen.findByTestId('approval-review-panel');
  await waitFor(() => expect(list).toHaveStyle({ minHeight: '188px' }));
  await user.click(screen.getByRole('button', { name: 'Approve options' }));
  await user.click(await screen.findByRole('menuitem', { name: label }));
  expect(screen.getByTestId('pending-approvals')).toBe(list);
  const confirm = screen.getByRole('button', { name: persistence === 'session' ? 'Approve for this session' : 'Always approve' });
  await waitFor(() => expect(confirm).toBeEnabled());
  expect(approve).not.toHaveBeenCalled();
  fireEvent.click(confirm);
  await waitFor(() => expect(approve).toHaveBeenCalledWith('pending', { persistence, params_pattern: pending.params_pattern }));
  expect(within(list).getByText('read_document')).toBeVisible();
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([second]);
  await act(async () => response.resolve({ id: 'pending', status: 'approved', persistence, approved_at: '2026-10-04T10:00:00Z' }));
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: 'write_document', exact: true })).toBeInTheDocument());
  expect(list).toHaveStyle({ minHeight: '188px' });
});

it('denies only on the deliberate Deny action and uses the separate permanent-denial API on confirmation', async () => {
  const response = Promise.withResolvers<{ id: string; status: 'denied'; denied_at: string }>();
  const deny = vi.spyOn(approvalApi, 'denyApproval').mockReturnValue(response.promise);
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  await user.click(screen.getByRole('button', { name: 'Deny options' }));
  expect(deny).not.toHaveBeenCalled();
  await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
  expect(deny).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
  const dialog = await screen.findByRole('dialog', { name: 'Confirm permanent denial' });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  expect(deny).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Deny options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Deny', exact: true }));
  await user.click(screen.getByRole('button', { name: 'Deny', exact: true }));
  await waitFor(() => expect(deny).toHaveBeenCalledWith('pending', {}));
  expect(screen.getByTestId('pending-approvals')).toBeVisible();
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  await act(async () => response.resolve({ id: 'pending', status: 'denied', denied_at: '2026-10-04T10:00:00Z' }));
  await screen.findByText("You're all caught up");
});

it('posts a permanent denial only after confirming its dialog', async () => {
  const deny = vi.spyOn(approvalApi, 'denyApproval').mockResolvedValue({ id: 'pending', status: 'denied', persistence: 'permanent', denied_at: '2026-10-04T10:00:00Z' });
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  await user.click(screen.getByRole('button', { name: 'Deny options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
  expect(deny).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
  fireEvent.click(within(await screen.findByRole('dialog', { name: 'Confirm permanent denial' })).getByRole('button', { name: 'Confirm permanent denial' }));
  await waitFor(() => expect(deny).toHaveBeenCalledWith('pending', { persistence: 'permanent' }));
});

it('expires an open editor at the server deadline without submitting a decision', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  vi.setSystemTime(new Date('2026-10-03T12:00:00Z'));
  const expiresAt = Date.now() + 4_000;
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([{ ...pending, expires_at: new Date(expiresAt).toISOString() }]);
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: 'pending', status: 'approved', persistence: 'once', approved_at: '2026-10-03T12:00:00Z' });
  renderPage();
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Approve options' }));
    await import('@components/approvals/ApprovalDecisionOptions');
    await vi.advanceTimersByTimeAsync(0);
  });
  fireEvent.click(screen.getByRole('menuitem', { name: 'For this session' }));
  await act(async () => { await vi.advanceTimersByTimeAsync(251); });
  expect(screen.getByRole('button', { name: 'Approve for this session' })).toBeEnabled();
  await act(async () => { await vi.advanceTimersByTimeAsync(expiresAt - Date.now()); });
  expect(screen.getByTestId('approval-outcome')).toBeVisible();
  const rowControl = screen.getByRole('button', { name: pending.tool_name });
  rowControl.focus();
  fireEvent.keyDown(rowControl, { key: 'a' });
  expect(approve).not.toHaveBeenCalled();
});

it('ignores inbox shortcuts when focus is on the body or an unrelated page component', async () => {
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  const approve = vi.spyOn(approvalApi, 'approveApproval');
  const deny = vi.spyOn(approvalApi, 'denyApproval');
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  const rowControl = screen.getByRole('button', { name: pending.tool_name });
  expect(document.body).toHaveFocus();
  await user.keyboard('adjk{Enter}');
  expect(within(panel()).getByRole('heading', { name: pending.tool_name, exact: true })).toBeInTheDocument();
  expect(panel()).not.toHaveFocus();
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();

  const outside = screen.getByTestId('approval-location');
  outside.tabIndex = 0;
  outside.focus();
  await user.keyboard('adjk{Enter}');
  expect(outside).toHaveFocus();
  for (const key of ['a', 'd', 'j', 'k', 'Enter']) fireEvent.keyDown(rowControl, { key });
  expect(outside).toHaveFocus();
  expect(within(panel()).getByRole('heading', { name: pending.tool_name, exact: true })).toBeInTheDocument();
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
});

it('uses focused J/K/Enter and A/D shortcuts without firing from modifiers or an open menu', async () => {
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: 'pending', status: 'approved', persistence: 'once', approved_at: '2026-10-04T10:00:00Z' });
  const deny = vi.spyOn(approvalApi, 'denyApproval').mockResolvedValue({ id: 'next', status: 'denied', denied_at: '2026-10-04T10:00:00Z' });
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  const rowControl = screen.getByRole('button', { name: pending.tool_name });
  rowControl.focus();
  await user.keyboard('j');
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: 'write_document', exact: true })).toBeInTheDocument());
  expect(rowControl).toHaveFocus();
  await user.keyboard('k');
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: 'read_document', exact: true })).toBeInTheDocument());
  for (const modifier of ['altKey', 'ctrlKey', 'metaKey', 'shiftKey', 'repeat']) {
    fireEvent.keyDown(rowControl, { key: 'a', [modifier]: true });
    fireEvent.keyDown(rowControl, { key: 'd', [modifier]: true });
  }
  const prevented = new KeyboardEvent('keydown', { key: 'a', bubbles: true, cancelable: true });
  prevented.preventDefault();
  fireEvent(rowControl, prevented);
  const reviewPanel = panel();
  await user.click(screen.getByRole('button', { name: 'Approve options' }));
  await screen.findByRole('menu');
  await user.keyboard('ad');
  rowControl.focus();
  for (const key of ['a', 'd', 'j', 'k', 'Enter']) fireEvent.keyDown(rowControl, { key });
  expect(within(reviewPanel).getByRole('heading', { name: pending.tool_name, exact: true, hidden: true })).toBeInTheDocument();
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
  await user.keyboard('{Escape}');
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
  rowControl.focus();
  await user.keyboard('{Enter}');
  expect(panel()).toHaveFocus();
  await user.keyboard('a');
  await waitFor(() => expect(approve).toHaveBeenCalledWith('pending', { persistence: 'once' }));
  expect(approve).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: 'write_document', exact: true })).toBeInTheDocument());
  screen.getByRole('button', { name: second.tool_name }).focus();
  await user.keyboard('d');
  await waitFor(() => expect(deny).toHaveBeenCalledWith('next', {}));
});

it('does not override editable controls inside the focused inbox', async () => {
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  const approve = vi.spyOn(approvalApi, 'approveApproval');
  const deny = vi.spyOn(approvalApi, 'denyApproval');
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  const inbox = screen.getByTestId('approval-section');
  const controls = [document.createElement('input'), document.createElement('textarea'), document.createElement('select'), document.createElement('div'), document.createElement('div')];
  controls[3].setAttribute('contenteditable', 'true');
  controls[4].setAttribute('role', 'combobox');
  for (const control of controls) {
    control.tabIndex = 0;
    inbox.append(control);
    control.focus();
    await user.keyboard('adjk{Enter}');
    expect(control).toHaveFocus();
    expect(within(panel()).getByRole('heading', { name: pending.tool_name, exact: true })).toBeInTheDocument();
    expect(approve).not.toHaveBeenCalled();
    expect(deny).not.toHaveBeenCalled();
    control.remove();
  }
});

it('ignores decision shortcuts in the remembered editor and permanent-denial dialog', async () => {
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: 'pending', status: 'approved', persistence: 'once', approved_at: '2026-10-04T10:00:00Z' });
  const deny = vi.spyOn(approvalApi, 'denyApproval').mockResolvedValue({ id: 'pending', status: 'denied', denied_at: '2026-10-04T10:00:00Z' });
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  await user.click(screen.getByRole('button', { name: 'Approve options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Always…' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Always approve' })).toBeEnabled());
  panel().focus();
  await user.keyboard('adjk{Enter}');
  expect(panel()).toHaveFocus();
  expect(within(panel()).getByRole('heading', { name: pending.tool_name, exact: true })).toBeInTheDocument();
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Approve options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Approve once' }));
  const reviewPanel = panel();
  await user.click(screen.getByRole('button', { name: 'Deny options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
  await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
  const dialog = await screen.findByRole('dialog', { name: 'Confirm permanent denial' });
  await user.keyboard('ad');
  expect(dialog).toContainElement(document.activeElement as HTMLElement);
  reviewPanel.focus();
  fireEvent.keyDown(reviewPanel, { key: 'a' });
  fireEvent.keyDown(reviewPanel, { key: 'd' });
  expect(approve).not.toHaveBeenCalled();
  expect(deny).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  expect(deny).not.toHaveBeenCalled();
});

it('filters remembered allow and deny decisions in URL, shows full pattern on demand, and confirms outline revoke', async () => {
  const allow = { ...pending, id: 'allow', status: 'approved' as const, persistence: 'permanent' as const };
  const denied = { ...pending, id: 'denied', status: 'denied' as const, persistence: 'permanent' as const, tool_name: 'delete_file' };
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([allow, denied]);
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([]);
  const revoke = vi.spyOn(approvalApi, 'revokePermanentApproval').mockResolvedValue({ id: denied.id, status: 'denied', denied_at: '2026-10-04T10:00:00Z' });
  const user = userEvent.setup();
  renderPage('/approvals/remembered');
  await screen.findByTestId('standing-decisions');
  expect(screen.getAllByTestId('standing-decision-row')).toHaveLength(2);
  await user.click(screen.getByRole('button', { name: `${copy.filter}: ${copy.all}` }));
  await user.click(within(await screen.findByRole('group', { name: copy.filter })).getByRole('button', { name: copy.denied }));
  expect(screen.getByTestId('approval-location')).toHaveTextContent('/approvals/remembered?filter=denied');
  await waitFor(() => expect(screen.getAllByTestId('standing-decision-row')).toHaveLength(1));
  const row = screen.getByTestId('standing-decision-row');
  expect(row).toHaveTextContent('delete_file');
  expect(row).toHaveAttribute('data-layout', 'stacked');
  expect(row.querySelector<HTMLElement>('[data-slot="entity-identity"]')).toContainElement(within(row).getByTestId('approval-agent-name'));
  expect(row.querySelector<HTMLElement>('[data-slot="entity-identity"]')).toContainElement(row.querySelector<HTMLElement>('[data-slot="entity-meta"]'));
  expect(within(row).getByTestId('entity-name')).toHaveTextContent('delete_file');
  expect(within(row).getByText('Always denied')).toBeVisible();
  const patternTrigger = within(row).getByRole('button', { name: 'View full scope pattern' });
  await user.click(patternTrigger);
  expect(screen.getByRole('dialog', { name: 'View full scope pattern' })).toBeVisible();
  expect(screen.getByText(denied.pattern_preview, { selector: '[data-slot="popover-content"] code' })).toBeVisible();
  await user.keyboard('{Escape}');
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'View full scope pattern' })).not.toBeInTheDocument());
  await waitFor(() => expect(patternTrigger).toHaveFocus());
  await user.click(within(row).getByRole('button', { name: 'Revoke', exact: true }));
  expect(revoke).not.toHaveBeenCalled();
  const dialog = await screen.findByRole('dialog', { name: 'Revoke remembered decision?' });
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([allow]);
  await user.click(within(dialog).getByRole('button', { name: 'Revoke decision' }));
  await waitFor(() => expect(revoke).toHaveBeenCalledWith(denied.id));
  await waitFor(() => expect(screen.queryByTestId('standing-decision-row')).not.toBeInTheDocument());
});

it('restores the remembered allow/deny filter from the URL', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([
    { ...pending, id: 'allow', status: 'approved', persistence: 'permanent' },
    { ...pending, id: 'deny', status: 'denied', persistence: 'permanent' },
  ]);
  renderPage('/approvals/remembered?filter=denied');
  const list = await screen.findByTestId('standing-decisions');
  expect(within(list).getAllByTestId('standing-decision-row')).toHaveLength(1);
  expect(within(list).getByTestId('standing-decision-row')).toHaveAttribute('id', 'deny');
  expect(screen.getByRole('button', { name: `${copy.filter}: ${copy.denied}` })).toBeVisible();
});

it('searches remembered tools, agents, and patterns without case and combines with URL filters', async () => {
  const allow = { ...pending, id: 'allow-hq', status: 'approved' as const, persistence: 'permanent' as const, tool_name: 'Export_Manifest', agent_display_name: 'Ledger Helper', tool_pattern: '*', pattern_preview: '*(path=/HQ)' };
  const other = { ...allow, id: 'allow-other', tool_name: 'read_file', agent_display_name: 'Research bot', pattern_preview: 'read(path=/else)' };
  const denied = { ...pending, id: 'denied-hq', status: 'denied' as const, persistence: 'permanent' as const, tool_name: 'remove_file', agent_display_name: 'Safety Helper', pattern_preview: 'remove(path=/HQ)' };
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([allow, other, denied]);
  const user = userEvent.setup();
  renderPage('/approvals/remembered?source=overview&filter=allowed');
  const list = await screen.findByTestId('standing-decisions');
  await user.click(screen.getByRole('button', { name: copy.searchRemembered }));
  const input = screen.getByRole('textbox', { name: copy.searchRemembered });
  for (const term of ['ExPoRt', 'LEDGER HELPER', '/hQ']) {
    await user.clear(input);
    await user.type(input, term);
    await waitFor(() => expect(within(list).getAllByRole('listitem').map(row => row.id)).toEqual([allow.id]));
  }
  expect(screen.getByTestId('approval-location')).toHaveTextContent('source=overview&filter=allowed&q=%2FhQ');
  await user.click(screen.getByRole('button', { name: `${copy.filter}: ${copy.allowed}` }));
  await user.click(within(await screen.findByRole('group', { name: copy.filter })).getByRole('button', { name: copy.denied }));
  await waitFor(() => expect(within(list).getAllByRole('listitem').map(row => row.id)).toEqual([denied.id]));
  expect(screen.getByTestId('approval-location')).toHaveTextContent('source=overview&filter=denied&q=%2FhQ');
  if (!screen.queryByRole('group', { name: copy.filter })) await user.click(screen.getByRole('button', { name: `${copy.filter}: ${copy.denied}` }));
  await user.click(within(await screen.findByRole('group', { name: copy.filter })).getByRole('button', { name: copy.all }));
  await waitFor(() => expect(within(list).getAllByRole('listitem').map(row => row.id)).toEqual([allow.id, denied.id]));
  expect(screen.getByTestId('approval-location')).toHaveTextContent('/approvals/remembered?source=overview&q=%2FhQ');
});

it('separates no matching remembered results from an empty remembered collection', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, status: 'approved', persistence: 'permanent' }]);
  const user = userEvent.setup();
  renderPage('/approvals/remembered?source=overview&q=not-present');
  const noResults = (await screen.findByRole('heading', { name: collectionCopy.noResults })).closest('[role="status"]')!;
  expect(screen.queryByText(copy.rememberedEmpty)).not.toBeInTheDocument();
  await user.click(within(noResults).getByRole('button', { name: collectionCopy.clearSearch }));
  expect(within(await screen.findByTestId('standing-decisions')).getAllByRole('listitem')).toHaveLength(1);
  expect(screen.getByTestId('approval-location')).toHaveTextContent('/approvals/remembered?source=overview');
});

it('keeps a long escaped revoke context bounded and expands only when measured as clipped', async () => {
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(60);
  const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(20);
  const toolName = `${'t'.repeat(320)}<img src=x onerror=alert(1)>`;
  const agentName = `${'a'.repeat(320)}<script>alert(2)</script>`;
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'denied', status: 'denied', persistence: 'permanent', tool_name: toolName, agent_display_name: agentName }]);
  renderPage('/approvals/remembered');
  fireEvent.click(await screen.findByRole('button', { name: 'Revoke', exact: true }));
  const dialog = await screen.findByRole('dialog', { name: 'Revoke remembered decision?' });
  const tool = within(dialog).getByText(toolName);
  const toolContext = tool.closest('[data-slot="truncated-text"]') as HTMLElement;
  const expand = within(toolContext).getByRole('button', { name: 'More' });
  const context = document.getElementById(expand.getAttribute('aria-controls')!);
  expect(context).toHaveClass('line-clamp-2');
  expect(context).toHaveTextContent(toolName);
  expect(dialog).toHaveTextContent(`for ${agentName}`);
  expect(dialog.querySelector('img,script')).toBeNull();
  fireEvent.click(expand);
  expect(within(toolContext).getByRole('button', { name: 'Less' })).toHaveAttribute('aria-expanded', 'true');
  expect(within(dialog).getByText('Future matching requests will need a new decision.')).toBeVisible();
  scroll.mockRestore(); height.mockRestore();
});

it('keeps stale pending and remembered rows and never calls an initial failure an empty success', async () => {
  renderPage();
  await screen.findByTestId('pending-approvals');
  vi.mocked(approvalApi.listPendingApprovals).mockRejectedValue(new Error('offline'));
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.pending('alice') }); });
  expect(screen.getByTestId('pending-approvals')).toHaveTextContent('read_document');
  expect(await screen.findByText('Pending approvals may be out of date.')).toBeVisible();
  cleanup(); client.clear();
  renderPage();
  await screen.findByText('Pending approvals could not be loaded.');
  expect(screen.queryByText("You're all caught up")).not.toBeInTheDocument();
  cleanup(); client.clear();
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'allow', status: 'approved', persistence: 'permanent' }]);
  renderPage('/approvals/remembered');
  await screen.findByTestId('standing-decisions');
  vi.mocked(approvalApi.listPermanentApprovals).mockRejectedValue(new Error('offline'));
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.standing('alice') }); });
  expect(screen.getByTestId('standing-decisions')).toHaveTextContent('read_document');
  expect(await screen.findByText('Remembered decisions may be out of date.')).toBeVisible();
});

it('shows loading without inventing rows and the truthful empty states only after server reads complete', async () => {
  const response = Promise.withResolvers<ToolApprovalDetail[]>();
  vi.mocked(approvalApi.listPendingApprovals).mockReturnValue(response.promise);
  renderPage();
  await screen.findByRole('status', { name: 'Loading pending approvals.' });
  expect(screen.queryByText("You're all caught up")).not.toBeInTheDocument();
  await act(async () => response.resolve([]));
  await screen.findByText("You're all caught up");
  fireEvent.click(screen.getByRole('tab', { name: /^Remembered/ }));
  await screen.findByText('No remembered decisions');
});

it('rejects a remembered scope when authoritative preview fails without posting approval', async () => {
  vi.mocked(approvalApi.previewApprovalScope).mockRejectedValue({ status: 422, code: 'invalid_pattern', message: 'Invalid pattern' });
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: 'pending', status: 'approved', persistence: 'permanent', approved_at: '2026-10-04T10:00:00Z' });
  const user = userEvent.setup();
  renderPage();
  await screen.findByTestId('approval-review-panel');
  await user.click(screen.getByRole('button', { name: 'Approve options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Always…' }));
  await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalledWith('pending', { params_pattern: pending.params_pattern }, expect.anything()));
  expect(screen.getByRole('button', { name: 'Always approve' })).toBeDisabled();
  expect(approve).not.toHaveBeenCalled();
});

it('announces arrivals and completed decisions without moving focus or adding another live region', async () => {
  vi.mocked(approvalApi.listPermanentApprovals).mockResolvedValue([{ ...pending, id: 'allow', tool_name: 'remembered_tool', status: 'approved', persistence: 'permanent' }]);
  renderPage();
  const first = within(await screen.findByTestId('pending-approvals')).getByRole('button');
  first.focus();
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([pending, second]);
  await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.pending('alice') }); });
  await waitFor(() => expect(screen.getAllByTestId('pending-approval-row')).toHaveLength(2));
  expect(screen.getByTestId('approval-announcement')).toHaveTextContent('1 new approval request');
  expect(document.activeElement).toBe(first);
  expect(document.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);
});

it('removes previous-principal rows and cannot decide a request belonging to another principal', async () => {
  const approve = vi.spyOn(approvalApi, 'approveApproval').mockResolvedValue({ id: 'pending', status: 'approved', persistence: 'once', approved_at: '2026-10-04T10:00:00Z' });
  renderPage();
  await screen.findByTestId('pending-approvals');
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
  vi.mocked(approvalApi.listPendingApprovals).mockResolvedValue([{ ...pending, principal: 'alice' }]);
  await act(async () => { await client.invalidateQueries({ queryKey: ['identity'] }); });
  await screen.findByText("You're all caught up");
  fireEvent.keyDown(document, { key: 'a' });
  expect(approve).not.toHaveBeenCalled();
  expect(screen.queryByTestId('pending-approvals')).not.toBeInTheDocument();
  expect(screen.getByTestId('approval-announcement')).toBeEmptyDOMElement();
});
