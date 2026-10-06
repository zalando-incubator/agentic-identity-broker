import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { approvalApi } from '@services/api/approvals';
import { accessCopy } from '@copy';
import { approvalCopy } from '@copy/approvals';
import { ApprovalReviewPage } from './ApprovalReviewPage';
import type { ToolApprovalDetail } from '../../types/approval';

vi.mock('@services/api/approvals', () => ({ approvalApi: { previewApprovalScope: vi.fn() } }));
const approval: ToolApprovalDetail = {
  id: 'approval-1', principal: 'alice@example.com', agent_id: 'agent-1',
  agent_display_name: 'Research Assistant', tool_name: 'read_file',
  arguments: { path: '<script>alert(1)</script>' }, tool_pattern: 'read_file',
  params_pattern: { path: '/tmp/example' }, pattern_preview: 'read_file(path=/tmp/example)',
  status: 'pending', approval_url: '/approvals/approval-1',
  created_at: '2026-03-29T00:00:00Z', expires_at: '2099-03-29T00:10:00Z',
};
function props(detail = approval) {
  return { approval: detail, actingPrincipal: 'alice@example.com', submitting: false, submittingAction: null as 'approve' | 'deny' | null,
    errorCode: null, errorMessage: null, approveResult: null, denyResult: null,
    onApprove: vi.fn().mockResolvedValue(undefined), onDeny: vi.fn().mockResolvedValue(undefined),
    onPreview: (pattern: Record<string, string>, signal: AbortSignal) => approvalApi.previewApprovalScope(detail.id, { params_pattern: pattern }, { signal }) };
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(approvalApi.previewApprovalScope).mockResolvedValue({ tool_pattern: 'read_file', params_pattern: { path: '/tmp/example' }, preview: 'read_file(path=/tmp/example)' });
});

describe('ApprovalReviewPage', () => {
  it('shows truthful identity, unrated risk, visible escaped arguments and GET scope without posting on a read-only visit', async () => {
    const user = userEvent.setup();
    const { container } = render(<ApprovalReviewPage {...props()} />);
    expect(screen.getByText('Research Assistant')).toBeVisible();
    expect(screen.getByText('alice@example.com')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'read_file', exact: true })).toBeVisible();
    expect(screen.getByText('Risk not rated')).toBeVisible();
    expect(screen.getByText(approval.pattern_preview)).toBeVisible();
    expect(screen.getByText('<script>alert(1)</script>')).toBeVisible();
    expect(container.querySelector('script')).toBeNull();
    expect(approvalApi.previewApprovalScope).not.toHaveBeenCalled();
    const trigger = screen.getByRole('button', { name: 'View JSON' });
    await user.click(trigger);
    const dialog = await screen.findByRole('dialog', { name: 'Raw tool arguments' });
    const raw = within(dialog).getByTestId('approval-arguments');
    expect(raw.textContent).toBe(JSON.stringify(approval.arguments, null, 2));
    expect(container.querySelector('script')).toBeNull();
    expect(dialog.querySelector('script')).toBeNull();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('button', { name: 'View JSON' })).toHaveFocus());
  });

  it('shows the full described inbox request once, with arguments first and no acting-user context or standalone shortcuts', () => {
    const description = '  Read the quarterly research report before drafting a summary.  ';
    const { rerender } = render(<ApprovalReviewPage {...props({ ...approval, description })} inboxSelected />);
    const panel = screen.getByTestId('approval-review-panel');
    expect(within(panel).getByRole('heading', { name: description.trim() })).toBeVisible();
    expect(within(panel).getAllByText('Research Assistant')).toHaveLength(1);
    expect(within(panel).getByTestId('approval-tool-name')).toHaveTextContent(approval.tool_name);
    expect(within(panel).queryByTestId('approval-acting-user')).not.toBeInTheDocument();
    expect(panel.querySelector('footer')?.previousElementSibling?.firstElementChild?.firstElementChild).toBe(within(panel).getByRole('region', { name: approvalCopy.arguments }));
    for (const [name, key] of [[accessCopy.deny, 'D'], [accessCopy.approveOnce, 'A']] as const) {
      const button = within(panel).getByRole('button', { name });
      expect(button.querySelector('kbd')).toHaveTextContent(key);
      expect(button.querySelector('kbd')).toHaveAttribute('aria-hidden', 'true');
      expect(button).not.toHaveAttribute('title');
    }
    rerender(<ApprovalReviewPage {...props({ ...approval, description: '   ' })} inboxSelected />);
    expect(within(panel).getByRole('heading', { name: approval.tool_name })).toBeVisible();
    rerender(<ApprovalReviewPage {...props()} />);
    expect(screen.getByTestId('approval-acting-user')).toHaveTextContent('alice@example.com');
    expect(screen.queryByRole('button', { name: 'Approve once' })?.querySelector('kbd')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Deny' })?.querySelector('kbd')).toBeNull();
  });

  it('shows requested and decided times relatively while keeping the expiry absolute', () => {
    const createdAt = new Date(Date.now() - 120_000).toISOString();
    const approvedAt = new Date(Date.now() - 60_000).toISOString();
    const detail = { ...approval, created_at: createdAt };
    const { rerender } = render(<ApprovalReviewPage {...props(detail)} />);
    expect(screen.getByText('Requested 2 minutes ago')).toHaveAttribute('datetime', createdAt);
    expect(screen.getByText(/^Expires:/)).toHaveAttribute('datetime', detail.expires_at);
    rerender(<ApprovalReviewPage {...props({ ...detail, status: 'approved', approved_at: approvedAt })} />);
    expect(screen.getByText('Approved 1 minute ago')).toHaveAttribute('datetime', approvedAt);
  });

  it('opens quiet session context beside technical details on demand and restores focus to its trigger', async () => {
    const user = userEvent.setup();
    render(<ApprovalReviewPage {...props({ ...approval, agent_session_id: 'agent-session-1', mcp_session_id: 'mcp-session-2', tool_invocation_id: 'invocation-3' })} />);
    const trigger = screen.getByRole('button', { name: 'Session context' });
    expect(screen.queryByText('agent-session-1')).not.toBeInTheDocument();
    await user.click(trigger);
    const context = await screen.findByRole('dialog', { name: 'Session context' });
    expect(within(context).getByText('agent-session-1')).toBeVisible();
    expect(within(context).getByText('mcp-session-2')).toBeVisible();
    expect(within(context).getByText('invocation-3')).toBeVisible();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Session context' })).toHaveFocus());
  });

  it('shows six argument values before decision and keeps extra nested values intact in JSON', async () => {
    const user = userEvent.setup();
    const detail = { ...approval, arguments: { first: 'one', second: true, third: 3, fourth: null, fifth: ['a'], sixth: { nested: 6 } } };
    const { container, rerender } = render(<ApprovalReviewPage {...props(detail)} />);
    expect(screen.getByText('first')).toBeVisible();
    expect(screen.getByText('one')).toBeVisible();
    expect(screen.getByText('sixth')).toBeVisible();
    expect(screen.getByText('{"nested":6}')).toBeVisible();
    expect(screen.queryByTestId('approval-arguments')).not.toBeInTheDocument();
    const expanded = { ...detail, arguments: { ...detail.arguments, seventh: { unsafe: '</pre><script>alert(1)</script>', lines: 'one\ntwo' } } };
    rerender(<ApprovalReviewPage {...props(expanded)} />);
    expect(screen.getByText('first')).toBeVisible();
    expect(screen.getByText('sixth')).toBeVisible();
    expect(screen.queryByText('seventh')).not.toBeInTheDocument();
    expect(screen.getByText('1 more argument in JSON')).toBeVisible();
    const trigger = screen.getByRole('button', { name: 'View JSON' });
    trigger.focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog', { name: 'Raw tool arguments' });
    const raw = within(dialog).getByTestId('approval-arguments');
    expect(raw.textContent).toBe(JSON.stringify(expanded.arguments, null, 2));
    expect(container.querySelector('script')).toBeNull();
    expect(dialog.querySelector('script')).toBeNull();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('button', { name: 'View JSON' })).toHaveFocus());
  });

  it('labels authoritative critical risk', () => {
    render(<ApprovalReviewPage {...props({ ...approval, risk_level: 'critical' })} />);
    expect(screen.getByTestId('approval-risk')).toHaveAccessibleDescription(/server/i);
  });

  it('keeps remembered scope unselected until the user chooses it', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} />);
    expect(screen.getByRole('button', { name: 'Approve once' })).toHaveAttribute('data-variant', 'primary');
    expect(screen.getByRole('button', { name: 'Deny', exact: true })).toHaveAttribute('data-variant', 'outline');
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    expect(await screen.findByRole('menuitem', { name: 'For this session' })).toBeVisible();
    await waitFor(() => expect(screen.getByRole('menuitem', { name: 'Approve once' })).toHaveFocus());
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Approve options' })).toHaveFocus());
    expect(callbacks.onApprove).not.toHaveBeenCalled();
    expect(screen.queryByRole('radio')).not.toBeInTheDocument();
  });

  it.each([['session', 'For this session', 'Approve for this session'], ['permanent', 'Always…', 'Always approve']] as const)('selects %s without opening its editor until the main action is clicked', async (persistence, option, action) => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: option }));
    expect(screen.queryByRole('button', { name: 'Confirm approval' })).not.toBeInTheDocument();
    expect(approvalApi.previewApprovalScope).not.toHaveBeenCalled();
    expect(callbacks.onApprove).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: action }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Confirm approval' }));
    expect(callbacks.onApprove).toHaveBeenCalledWith({ persistence, params_pattern: approval.params_pattern });
  });

  it('selects permanent denial without opening confirmation and can switch back to deny once', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Deny options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(callbacks.onDeny).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'Deny options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Deny', exact: true }));
    await user.click(screen.getByRole('button', { name: 'Deny', exact: true }));
    expect(callbacks.onDeny).toHaveBeenCalledWith();
  });

  it('does not reopen a decision menu when returning from the scope editor', async () => {
    const user = userEvent.setup();
    render(<ApprovalReviewPage {...props()} />);
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'For this session' }));
    await user.click(screen.getByRole('button', { name: 'Approve for this session' }));
    await user.click(screen.getByRole('button', { name: 'Back' }));
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve for this session' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Deny options' })).toBeVisible();
  });

  it('keeps rejected patterns editable and blocks confirmation until server validation succeeds', async () => {
    const user = userEvent.setup();
    vi.mocked(approvalApi.previewApprovalScope).mockRejectedValue({ status: 422, code: 'invalid_pattern', message: 'Invalid pattern' });
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} errorCode="INVALID_PATTERN" />);
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'For this session' }));
    await user.click(screen.getByRole('button', { name: 'Approve for this session' }));
    await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalled());
    expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    expect(screen.getByRole('combobox', { name: 'Path match mode' })).toBeEnabled();
    expect(callbacks.onApprove).not.toHaveBeenCalled();
  });

  it('resets remembered persistence and custom pattern when a cached route changes ID', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    const { rerender } = render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Always…' }));
    await user.click(screen.getByRole('button', { name: 'Always approve' }));
    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    await user.click(screen.getByRole('combobox', { name: 'Path match mode' }));
    await user.click(screen.getByRole('option', { name: 'Any value' }));
    const next = { ...approval, id: 'approval-2', params_pattern: { path: '/new' } };
    rerender(<ApprovalReviewPage {...callbacks} approval={next} onPreview={(pattern, signal) => approvalApi.previewApprovalScope(next.id, { params_pattern: pattern }, { signal })} />);
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    expect(callbacks.onApprove).toHaveBeenCalledWith({ persistence: 'once' });
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'For this session' }));
    await user.click(screen.getByRole('button', { name: 'Approve for this session' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Confirm approval' }));
    expect(callbacks.onApprove).toHaveBeenLastCalledWith({ persistence: 'session', params_pattern: { path: '/new' } });
  });

  it('requires permanent-denial confirmation and returns focus when canceled', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} />);
    const trigger = screen.getByRole('button', { name: 'Deny options' });
    await user.click(trigger);
    await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
    await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
    const dialog = await screen.findByRole('dialog', { name: 'Confirm permanent denial' });
    expect(callbacks.onDeny).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Always deny', exact: true })).toHaveFocus());
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(callbacks.onDeny).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Deny options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
    await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Confirm permanent denial' }));
    expect(callbacks.onDeny).toHaveBeenCalledWith(true);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('closes outgoing decision portals and rejects clicks after the selected request changes', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    const { rerender } = render(<ApprovalReviewPage {...callbacks} inboxSelected />);
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    expect(await screen.findByRole('menuitem', { name: 'Always…' })).toBeVisible();

    rerender(<ApprovalReviewPage {...callbacks} inboxSelected decisionEnabled={false} />);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    await user.click(screen.getByRole('button', { name: 'Deny', exact: true }));
    expect(callbacks.onApprove).not.toHaveBeenCalled();
    expect(callbacks.onDeny).not.toHaveBeenCalled();

    rerender(<ApprovalReviewPage {...callbacks} inboxSelected decisionEnabled />);
    await user.click(screen.getByRole('button', { name: 'Deny options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Always deny…' }));
    await user.click(screen.getByRole('button', { name: 'Always deny', exact: true }));
    expect(await screen.findByRole('dialog', { name: 'Confirm permanent denial' })).toBeVisible();
    rerender(<ApprovalReviewPage {...callbacks} inboxSelected decisionEnabled={false} />);
    expect(screen.queryByRole('dialog', { name: 'Confirm permanent denial' })).not.toBeInTheDocument();
    expect(callbacks.onDeny).not.toHaveBeenCalled();
  });

  it.each(['approved', 'denied', 'expired'] as const)('renders %s without decision controls', (status) => {
    const detail = status === 'expired' ? { ...approval, expires_at: '2000-01-01T00:00:00Z' } : { ...approval, status };
    render(<ApprovalReviewPage {...props(detail)} />);
    expect(screen.queryByRole('button', { name: /approve|deny/i })).not.toBeInTheDocument();
    expect(screen.getByRole(status === 'expired' ? 'alert' : 'status')).toBeVisible();
  });

  it.each(['approved', 'denied'] as const)('bounds unbroken names in the %s outcome and expands escaped text by keyboard', async (status) => {
    const user = userEvent.setup();
    const toolName = `${'t'.repeat(320)}<img src=x onerror=alert(1)>`;
    const agentName = `${'a'.repeat(320)}<script>alert(2)</script>`;
    const scroll = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(60);
    const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(20);
    render(<ApprovalReviewPage {...props({ ...approval, status, tool_name: toolName, agent_display_name: agentName })} />);
    const outcome = screen.getByRole('status');
    const tool = within(outcome).getByText(toolName);
    const toolContext = tool.closest('[data-slot="truncated-text"]') as HTMLElement;
    const expand = within(toolContext).getByRole('button', { name: 'More' });
    const context = document.getElementById(expand.getAttribute('aria-controls')!);
    expect(context).toHaveClass('line-clamp-2');
    expect(context).toHaveTextContent(toolName);
    expect(outcome).toHaveTextContent(`for ${agentName}`);
    expect(outcome.querySelector('img, script')).toBeNull();
    await user.tab();
    expect(expand).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(within(toolContext).getByRole('button', { name: 'Less' })).toHaveAttribute('aria-expanded', 'true');
    expect(context).not.toHaveClass('line-clamp-2');
    await user.keyboard(' ');
    expect(expand).toHaveAttribute('aria-expanded', 'false');
    expect(context).toHaveClass('line-clamp-2');
    scroll.mockRestore();
    height.mockRestore();
  });

  it.each(['approved', 'denied'] as const)('keeps the conflict notice alongside the authoritative %s outcome without offering actions', async (status) => {
    const user = userEvent.setup();
    const callbacks = props();
    const { rerender } = render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    rerender(<ApprovalReviewPage {...callbacks} approval={{ ...approval, status }} errorCode="ALREADY_ACTIONED" />);
    expect(screen.getByRole('alert')).toBeVisible();
    expect(within(screen.getByRole('status')).getByRole('heading', { name: status === 'approved' ? 'Approved' : 'Denied' })).toBeVisible();
    expect(screen.getByTestId('approval-outcome')).toHaveTextContent(status === 'approved' ? 'Approved' : 'Denied');
    expect(screen.queryByRole('button', { name: /approve|deny/i })).not.toBeInTheDocument();
  });

  it('keeps network failure non-successful and leaves a deliberate decision available', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} errorCode="NETWORK_ERROR" errorMessage="Unable to connect" />);
    expect(within(screen.getByRole('alert')).getByText('Unable to connect')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    expect(callbacks.onApprove).toHaveBeenCalledWith({ persistence: 'once' });
  });
});
