import { useState, type ComponentProps } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { useLocation } from 'react-router-dom';
import type { ScopePreview, ToolApprovalDetail } from '../types/approval';
import { commonCopy, navigationCopy } from '@copy';
import { ApprovalsView } from './ApprovalsView';
import {
  calendarRequest, criticalRequest, denseRequests, exactApprovalPreview, expiredRequest, fileRequest,
  longApproval, pendingRequests, rememberedAllow, rememberedDeny, rememberedRequests, storyNow,
} from '../storybook/fixtures';
import { ConsoleStoryShell } from '../storybook/ScreenShell';

const calendarPreview = fn(exactApprovalPreview(calendarRequest));
const filePreview = fn(exactApprovalPreview(fileRequest));
const criticalPreview = fn(exactApprovalPreview(criticalRequest));
type PreviewCallback = (paramsPattern: Record<string, string>, signal: AbortSignal) => Promise<ScopePreview>;
const previewById: Record<string, PreviewCallback> = {
  [calendarRequest.id]: calendarPreview,
  [fileRequest.id]: filePreview,
  [criticalRequest.id]: criticalPreview,
};

type ViewProps = ComponentProps<typeof ApprovalsView>;
const defaultArgs: ViewProps = {
  tab: 'pending',
  pending: { data: pendingRequests, loading: false, stale: false, onRetry: fn() },
  standing: { data: rememberedRequests, loading: false, stale: false, onRetry: fn() },
  selectedId: calendarRequest.id,
  onSelect: fn(),
  onApproveOnce: fn(),
  onDenyOnce: fn(),
  review: {
    approval: calendarRequest,
    actingPrincipal: calendarRequest.principal,
    submittingAction: null,
    submitting: false,
    errorCode: null,
    errorMessage: null,
    approveResult: null,
    denyResult: null,
    onApprove: fn(async () => {}),
    onDeny: fn(async () => {}),
    onPreview: calendarPreview,
    onRetry: fn(),
  },
  search: '',
  onSearchChange: fn(),
  filter: 'all',
  onFilterChange: fn(),
  revoke: { confirmation: null, isPending: fn(() => false), error: false, request: fn(), cancel: fn(), confirm: fn() },
};

async function assertDecisionFits(canvasElement: HTMLElement, request: ToolApprovalDetail, rowIndex: number) {
  const canvas = within(canvasElement);
  const row = canvas.getAllByTestId('pending-approval-row')[rowIndex]!;
  const title = within(row).getByTestId('entity-name');
  await expect(row).toHaveAttribute('data-layout', 'stacked');
  await expect(title).toHaveTextContent(request.tool_name);
  await expect(within(row).getByTestId('approval-agent-name')).toHaveTextContent(request.agent_display_name ?? request.agent_id);
  await expect(row.querySelector<HTMLElement>('[data-slot="entity-identity"]')).toContainElement(title);
  await expect(row.querySelector<HTMLElement>('[data-slot="entity-identity"]')).toContainElement(row.querySelector<HTMLElement>('[data-slot="entity-status"]'));
  if (window.innerWidth < 1012) {
    await expect(within(row).getByRole('link')).toHaveAttribute('href', `/approvals/${request.id}`);
    await expect(within(row).getByRole('link')).not.toHaveAttribute('aria-controls');
    return;
  }
  await expect(within(canvas.getByTestId('selected-approval-identity')).getByRole('heading', { name: request.description?.trim() || request.tool_name })).toBeVisible();
  await expect(canvas.getByTestId('selected-approval-identity')).toHaveTextContent(request.agent_display_name ?? request.agent_id);
  await expect(within(row).getByRole('button', { name: request.tool_name })).toHaveAttribute('aria-controls', `approval-review-${request.id}`);
  if (window.innerWidth !== 1280) return;
  expect(window.innerHeight).toBe(720);
  await expect(row.getBoundingClientRect().height).toBe(88);
  await expect(title.getBoundingClientRect().width).toBeGreaterThanOrEqual(260);
  const panel = canvas.getByTestId('approval-review-panel').getBoundingClientRect();
  for (const name of ['Deny', 'Approve once']) {
    const action = canvas.getByRole('button', { name, exact: true }).getBoundingClientRect();
    await expect(action.left).toBeGreaterThanOrEqual(0);
    await expect(action.right).toBeLessThanOrEqual(window.innerWidth);
    await expect(action.top).toBeGreaterThanOrEqual(0);
    await expect(action.bottom).toBeLessThanOrEqual(window.innerHeight);
    await expect(action.left).toBeGreaterThanOrEqual(panel.left);
    await expect(action.right).toBeLessThanOrEqual(panel.right);
    await expect(action.top).toBeGreaterThanOrEqual(panel.top);
    await expect(action.bottom).toBeLessThanOrEqual(panel.bottom);
  }
}

// Tabs follow the story router; selection, search, filter, and dialog visibility are local state.
// Approval and revoke callbacks stay as supplied by the story args: no API or fabricated outcome.
function InboxStory(args: ViewProps) {
  const tab = useLocation().pathname === '/approvals/remembered' ? 'remembered' : 'pending';
  const [selectedId, setSelectedId] = useState(args.selectedId);
  const [filter, setFilter] = useState(args.filter);
  const [search, setSearch] = useState(args.search);
  const [confirmation, setConfirmation] = useState(args.revoke.confirmation);
  const selected = args.pending.data?.find((request) => request.id === selectedId);
  const review = selected && args.review
    ? { ...args.review, approval: selected, onPreview: previewById[selected.id] ?? args.review.onPreview }
    : args.review;
  return <ApprovalsView {...args} tab={tab} compact={args.compact ?? window.matchMedia('(max-width: 1011px)').matches} selectedId={selectedId} search={search} filter={filter} review={tab === 'pending' ? review : null}
    onSelect={(id) => { args.onSelect(id); setSelectedId(id); }}
    onSearchChange={(value) => { args.onSearchChange(value); setSearch(value); }}
    onFilterChange={(value) => { args.onFilterChange(value); setFilter(value); }}
    revoke={{
      ...args.revoke,
      confirmation,
      request: (approval, trigger) => { args.revoke.request(approval, trigger); setConfirmation(approval); },
      cancel: () => { args.revoke.cancel(); setConfirmation(null); },
      confirm: () => { args.revoke.confirm(); },
    }} />;
}

const meta = {
  title: 'Screens/Approvals',
  component: ApprovalsView,
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story, context) => <ConsoleStoryShell path={context.args.tab === 'remembered' ? '/approvals/remembered' : '/approvals'}><Story /></ConsoleStoryShell>],
  render: args => <InboxStory {...args} />,
  args: defaultArgs,
} satisfies Meta<typeof ApprovalsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = { args: { pending: { ...defaultArgs.pending, data: undefined, loading: true }, selectedId: null, review: null } };
export const RememberedLoading: Story = { args: { tab: 'remembered', standing: { ...defaultArgs.standing, data: undefined, loading: true }, selectedId: null, review: null } };
export const RememberedEmpty: Story = { args: { tab: 'remembered', standing: { ...defaultArgs.standing, data: [] }, selectedId: null, review: null } };
export const Empty: Story = { args: { pending: { ...defaultArgs.pending, data: [] }, selectedId: null, review: null } };
export const Typical: Story = {
  play: async ({ canvasElement }) => {
    await assertDecisionFits(canvasElement, calendarRequest, 0);
  },
};

export const ExpiringSoon: Story = {
  args: {
    pending: { ...defaultArgs.pending, data: [{ ...calendarRequest, expires_at: new Date(storyNow + 4 * 60_000).toISOString() }, fileRequest] },
  },
  play: async ({ canvasElement }) => {
    const rows = within(canvasElement).getAllByTestId('pending-approval-row');
    await expect(within(rows[0]!).getByText('4 min left')).toBeVisible();
    await expect(within(rows[0]!).getByText('4 min left')).toHaveAttribute('title', 'Expires in 4 minutes');
    if (window.innerWidth >= 1012) {
      const detail = within(canvasElement).getByRole('region', { name: /^Selected request:/ });
      await expect(within(detail).getByRole('time')).toHaveAttribute('datetime', new Date(storyNow + 4 * 60_000).toISOString());
      await expect(within(detail).getByRole('time')).toHaveTextContent('Expires:');
    } else {
      await expect(within(rows[0]!).getByRole('link')).toHaveAttribute('href', `/approvals/${calendarRequest.id}`);
      await expect(within(canvasElement).queryByRole('region', { name: /^Selected request:/ })).not.toBeInTheDocument();
    }
    await expect(within(rows[1]!).queryByText(/min left/)).not.toBeInTheDocument();
    await expect(rows.map(row => row.getBoundingClientRect().height)).toEqual([88, 88]);
  },
};
export const Dense: Story = {
  args: { pending: { ...defaultArgs.pending, data: denseRequests }, selectedId: denseRequests[0].id, review: { ...defaultArgs.review!, approval: denseRequests[0] } },
  play: async ({ canvasElement }) => {
    const rows = within(canvasElement).getAllByTestId('pending-approval-row');
    await expect(rows).toHaveLength(13);
    await expect(rows.every(row => row.getBoundingClientRect().height === 88)).toBe(true);
  },
};
export const Error: Story = {
  args: { pending: { ...defaultArgs.pending, data: undefined, stale: true }, selectedId: null, review: null },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole('alert')).toBeVisible();
    await userEvent.click(canvas.getByRole('button', { name: commonCopy.retry }));
    await expect(args.pending.onRetry).toHaveBeenCalledOnce();
  },
};
export const Stale: Story = {
  args: { pending: { ...defaultArgs.pending, stale: true } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole('alert')).toBeVisible();
    await expect(canvas.getAllByTestId('pending-approval-row')).toHaveLength(pendingRequests.length);
    await assertDecisionFits(canvasElement, calendarRequest, 0);
  },
};
export const HighRiskSelected: Story = {
  args: { selectedId: criticalRequest.id, review: { ...defaultArgs.review!, approval: criticalRequest, onPreview: criticalPreview } },
  play: async ({ canvasElement }) => {
    await assertDecisionFits(canvasElement, criticalRequest, 2);
  },
};
export const ExpiredRequest: Story = {
  args: {
    pending: { ...defaultArgs.pending, data: [expiredRequest] },
    review: { ...defaultArgs.review!, approval: expiredRequest },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    if (window.innerWidth < 1012) {
      await expect(within(canvas.getByTestId('pending-approval-row')).getByRole('link')).toHaveAttribute('href', `/approvals/${expiredRequest.id}`);
      return;
    }
    await expect(canvas.getByTestId('approval-outcome')).toBeVisible();
    await expect(canvas.queryByRole('button', { name: /approve once/i })).not.toBeInTheDocument();
  },
};
export const RememberedScopeEditor: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const list = canvas.getByTestId('pending-approvals');
    if (window.innerWidth < 1012) {
      await expect(within(canvas.getAllByTestId('pending-approval-row')[0]).getByRole('link')).toHaveAttribute('href', `/approvals/${calendarRequest.id}`);
      return;
    }
    const before = list.getBoundingClientRect();
    await userEvent.click(canvas.getByRole('button', { name: 'Approve options' }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('menuitem', { name: 'Always…' }));
    await waitFor(() => expect(canvas.getByRole('button', { name: 'Confirm approval' })).toBeEnabled());
    await expect(args.review?.onPreview).toHaveBeenCalled();
    await userEvent.click(canvas.getByRole('button', { name: 'Confirm approval' }));
    await expect(args.review?.onApprove).toHaveBeenCalledWith({ persistence: 'permanent', params_pattern: calendarRequest.params_pattern });
    const after = list.getBoundingClientRect();
    await expect([after.x, after.y, after.width, after.height]).toEqual([before.x, before.y, before.width, before.height]);
    await expect(canvas.getByRole('button', { name: 'Back' })).toBeVisible();
  },
};
export const RememberedTab: Story = {
  args: { tab: 'remembered', selectedId: null, review: null },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const rows = canvas.getAllByTestId('standing-decision-row');
    await expect(rows).toHaveLength(rememberedRequests.length);
    await expect(canvas.getByRole('tab', { name: 'Remembered' })).toHaveAttribute('aria-selected', 'true');
    for (const [index, row] of rows.entries()) {
      await expect(row).toHaveAttribute('data-layout', 'stacked');
      await expect(within(row).getByTestId('entity-name')).toHaveTextContent(rememberedRequests[index]!.tool_name);
      await expect(row.querySelector<HTMLElement>('[data-slot="entity-identity"]')).toContainElement(within(row).getByTestId('approval-agent-name'));
      await expect(row.getBoundingClientRect().height).toBe(window.innerWidth < 768 ? 160 : 88);
    }
    if (window.innerWidth === 1280) await expect(within(rows[0]!).getByTestId('entity-name').getBoundingClientRect().width).toBeGreaterThanOrEqual(500);
  },
};

export const TabGeometryAndControls: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const heading = canvas.getByRole('heading', { level: 1, name: navigationCopy.approvals });
    const tabList = canvas.getByRole('tablist', { name: navigationCopy.approvals });
    const bounds = (element: HTMLElement) => {
      const { x, y, width, height } = element.getBoundingClientRect();
      return [x, y, width, height];
    };
    const before = [bounds(heading), bounds(tabList)];

    const rememberedTab = canvas.getByRole('tab', { name: 'Remembered' });
    await userEvent.click(rememberedTab);
    const remembered = canvas.getByRole('tabpanel', { name: 'Remembered' });
    await expect(canvas.getByRole('tab', { name: 'Remembered' })).toHaveAttribute('aria-selected', 'true');
    await expect([bounds(heading), bounds(tabList)]).toEqual(before);
    await expect(canvas.queryByRole('button', { name: 'Approve once' })).not.toBeInTheDocument();
    await userEvent.click(within(remembered).getByRole('button', { name: 'Filter decisions: All' }));
    await userEvent.click(within(await within(canvasElement.ownerDocument.body).findByRole('group', { name: 'Filter decisions' })).getByRole('button', { name: 'Always denied' }));
    await waitFor(() => expect(within(remembered).getAllByRole('listitem').map(row => row.id)).toEqual([rememberedDeny.id]));
    await expect(args.onFilterChange).toHaveBeenCalledWith('denied');
    await userEvent.click(within(remembered).getByRole('button', { name: 'Search tools, agents, or patterns' }));
    const search = within(remembered).getByRole('textbox', { name: 'Search tools, agents, or patterns' });
    await expect(search).toHaveFocus();
    await userEvent.type(search, rememberedDeny.tool_name);
    await waitFor(() => expect(within(remembered).getAllByRole('listitem').map(row => row.id)).toEqual([rememberedDeny.id]));
    await expect(args.onSearchChange).toHaveBeenCalledWith(rememberedDeny.tool_name);
    await expect([bounds(heading), bounds(tabList)]).toEqual(before);

    await userEvent.click(canvas.getByRole('tab', { name: /^Pending/ }));
    await expect(canvas.getByRole('tab', { name: /^Pending/ })).toHaveAttribute('aria-selected', 'true');
    await expect(canvas.queryByRole('button', { name: 'Filter decisions: Always denied' })).not.toBeInTheDocument();
    await expect(canvas.getAllByTestId('pending-approval-row')).toHaveLength(pendingRequests.length);
    await expect([bounds(heading), bounds(tabList)]).toEqual(before);
  },
};
export const LongRememberedContent: Story = {
  args: {
    tab: 'remembered', selectedId: null, review: null,
    standing: { ...defaultArgs.standing, data: [{ ...rememberedAllow, id: 'long-remembered', approval_url: '/approvals/long-remembered', agent_display_name: longApproval.agent_display_name, tool_name: longApproval.tool_name, tool_pattern: longApproval.tool_pattern, params_pattern: longApproval.params_pattern, pattern_preview: longApproval.pattern_preview }] },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = canvas.getByTestId('standing-decision-row');
    await expect(row.getBoundingClientRect().height).toBe(window.innerWidth < 768 ? 160 : 88);
    await expect(within(row).getByTestId('entity-name')).toHaveTextContent(longApproval.tool_name);
    await expect(within(row).getByTestId('approval-agent-name')).toHaveAttribute('title', longApproval.agent_display_name);
    await expect(within(row).getByRole('button', { name: 'View full scope pattern' })).toBeVisible();
    const revoke = within(row).getByRole('button', { name: 'Revoke' });
    await expect(revoke.getBoundingClientRect().right).toBeLessThanOrEqual(row.getBoundingClientRect().right);
  },
};
export const RememberedFiltered: Story = {
  args: { tab: 'remembered', filter: 'allowed', selectedId: null, review: null },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    // Exiting Motion rows remain in the DOM but are inert; count accessible decisions, not test IDs.
    const list = within(canvas.getByTestId('standing-decisions'));
    await expect(list.getAllByRole('listitem')).toHaveLength(1);
    await userEvent.click(canvas.getByRole('button', { name: 'Filter decisions: Always allowed' }));
    const options = await within(canvasElement.ownerDocument.body).findByRole('group', { name: 'Filter decisions' });
    await userEvent.click(within(options).getByRole('button', { name: 'Always denied' }));
    await expect(args.onFilterChange).toHaveBeenCalledWith('denied');
    await waitFor(() => {
      const rows = list.getAllByRole('listitem');
      expect(rows).toHaveLength(1);
      expect(rows[0]).toHaveAttribute('id', rememberedDeny.id);
    });
    // Let the inert row finish its exit before the screenshot captures the filtered list.
    await waitFor(() => expect(canvas.getByTestId('standing-decisions').querySelector('[data-exiting="true"]')).toBeNull());
  },
};
export const RememberedSearch: Story = {
  args: { tab: 'remembered', search: 'TEAM-OPS', filter: 'allowed', selectedId: null, review: null },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Search tools, agents, or patterns: TEAM-OPS', exact: true }));
    await expect(canvas.getByRole('textbox', { name: 'Search tools, agents, or patterns' })).toHaveValue('TEAM-OPS');
    const rows = within(canvas.getByTestId('standing-decisions')).getAllByRole('listitem');
    await expect(rows).toHaveLength(1);
    await expect(rows[0]).toHaveAttribute('id', rememberedAllow.id);
  },
};
export const RememberedNoResults: Story = {
  args: { tab: 'remembered', search: 'missing tool', filter: 'denied', selectedId: null, review: null },
};
export const RevokeWithConfirmation: Story = {
  args: { tab: 'remembered', selectedId: null, review: null },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getAllByRole('button', { name: /revoke/i })[0]);
    const dialog = await within(canvasElement.ownerDocument.body).findByRole('dialog');
    await expect(args.revoke.request).toHaveBeenCalledWith(rememberedAllow, expect.any(HTMLButtonElement));
    await expect(args.revoke.confirm).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Revoke decision' }));
    await expect(args.revoke.confirm).toHaveBeenCalledOnce();
  },
};
export const KeyboardAndListStability: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const selectedDetail = () => canvas.getByRole('region', { name: /^Selected request:/ });
    const list = canvas.getByTestId('pending-approvals');
    if (window.innerWidth < 1012) {
      await expect(within(canvas.getAllByTestId('pending-approval-row')[0]).getByRole('link')).toHaveAttribute('href', `/approvals/${calendarRequest.id}`);
      return;
    }
    const active = canvasElement.ownerDocument.activeElement;
    if (active instanceof HTMLElement) active.blur();
    await expect(canvasElement.ownerDocument.body).toHaveFocus();
    await userEvent.keyboard('adjk{Enter}');
    await expect(args.onSelect).not.toHaveBeenCalled();
    await expect(args.onApproveOnce).not.toHaveBeenCalled();
    await expect(args.onDenyOnce).not.toHaveBeenCalled();
    const pendingTab = canvas.getByRole('tab', { name: /Pending/ });
    pendingTab.focus();
    await userEvent.keyboard('adjk{Enter}');
    await expect(args.onSelect).not.toHaveBeenCalled();
    await expect(args.onApproveOnce).not.toHaveBeenCalled();
    await expect(args.onDenyOnce).not.toHaveBeenCalled();
    const before = list.getBoundingClientRect();
    const firstRow = canvas.getAllByTestId('pending-approval-row')[0];
    const firstTitleLeft = within(firstRow).getByTestId('entity-name').getBoundingClientRect().left;
    const rowControl = within(firstRow).getByRole('button');
    rowControl.focus();
    await userEvent.keyboard('j');
    const fileRow = canvas.getAllByTestId('pending-approval-row')[1];
    await waitFor(() => expect(fileRow).toHaveAttribute('data-selected', 'true'));
    await expect(within(firstRow).getByTestId('entity-name').getBoundingClientRect().left).toBe(firstTitleLeft);
    await expect(within(fileRow).getByTestId('entity-name').getBoundingClientRect().left).toBe(firstTitleLeft);
    await expect(within(fileRow).getByRole('button', { name: fileRequest.tool_name })).toHaveAttribute('aria-controls', `approval-review-${fileRequest.id}`);
    await expect(within(selectedDetail()).getByTestId('selected-approval-identity')).toHaveTextContent(fileRequest.description!);
    await expect(within(selectedDetail()).getByTestId('selected-approval-identity')).toHaveTextContent(fileRequest.agent_display_name!);
    const nextBounds = list.getBoundingClientRect();
    await expect([nextBounds.x, nextBounds.y, nextBounds.width, nextBounds.height]).toEqual([before.x, before.y, before.width, before.height]);
    await waitFor(() => expect(args.onSelect).toHaveBeenCalledWith(fileRequest.id));
    await userEvent.keyboard('k');
    await waitFor(() => expect(args.onSelect).toHaveBeenLastCalledWith(calendarRequest.id));
    await expect(firstRow).toHaveAttribute('data-selected', 'true');
    await expect(fileRow).not.toHaveAttribute('data-selected');
    await expect(within(selectedDetail()).getByTestId('selected-approval-identity')).toHaveTextContent(calendarRequest.description!);
    await userEvent.keyboard('a');
    await expect(args.onApproveOnce).toHaveBeenCalledOnce();
    await userEvent.keyboard('d');
    await expect(args.onDenyOnce).toHaveBeenCalledOnce();
    await userEvent.keyboard('{Enter}');
    await expect(selectedDetail()).toHaveFocus();
    await userEvent.click(canvas.getByRole('button', { name: 'Approve options' }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('menuitem', { name: 'Always…' }));
    await waitFor(() => expect(canvas.getByRole('button', { name: 'Confirm approval' })).toBeEnabled());
    selectedDetail().focus();
    await userEvent.keyboard('adjk{Enter}');
    await expect(args.onApproveOnce).toHaveBeenCalledOnce();
    await expect(args.onDenyOnce).toHaveBeenCalledOnce();
    await expect(args.onSelect).toHaveBeenLastCalledWith(calendarRequest.id);
    const after = list.getBoundingClientRect();
    await expect([after.x, after.y, after.width, after.height]).toEqual([before.x, before.y, before.width, before.height]);
  },
};
export const LongContent: Story = {
  args: {
    pending: { ...defaultArgs.pending, data: [longApproval, ...pendingRequests] },
    selectedId: longApproval.id,
    review: { ...defaultArgs.review!, approval: longApproval, onPreview: fn(exactApprovalPreview(longApproval)) },
  },
  play: async ({ canvasElement }) => {
    await assertDecisionFits(canvasElement, longApproval, 0);
    const canvas = within(canvasElement);
    const row = canvas.getAllByTestId('pending-approval-row')[0];
    const title = within(row).getByTestId('entity-name');
    const measure = title.querySelector('[data-measure-text]')!;
    const clipped = measure.scrollHeight > measure.clientHeight;
    const control = within(row).getByRole(window.innerWidth < 1012 ? 'link' : 'button', { name: longApproval.tool_name });
    await userEvent.hover(control);
    if (clipped) await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toHaveTextContent(longApproval.tool_name);
    else await expect(within(canvasElement.ownerDocument.body).queryByRole('tooltip')).not.toBeInTheDocument();
    await userEvent.unhover(control);
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).queryByRole('tooltip')).not.toBeInTheDocument());
  },
};
