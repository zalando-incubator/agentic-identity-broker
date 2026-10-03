import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { approvalApi } from '@services/api/approvals';
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
  return { approval: detail, actingPrincipal: 'alice@example.com', submitting: false,
    errorCode: null, errorMessage: null, approveResult: null, denyResult: null,
    onApprove: vi.fn().mockResolvedValue(undefined), onDeny: vi.fn().mockResolvedValue(undefined) };
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(approvalApi.previewApprovalScope).mockResolvedValue({ tool_pattern: 'read_file', params_pattern: { path: '/tmp/example' }, preview: 'read_file(path=/tmp/example)' });
});

describe('ApprovalReviewPage', () => {
  it('shows truthful identity, unrated risk and GET scope without posting on a read-only visit', async () => {
    const user = userEvent.setup();
    const { container } = render(<ApprovalReviewPage {...props()} />);
    expect(screen.getByText('Research Assistant')).toBeVisible();
    expect(screen.getByText('alice@example.com')).toBeVisible();
    expect(screen.getByText('read_file', { selector: 'h3' })).toBeVisible();
    expect(screen.getByText('Risk not rated')).toBeVisible();
    expect(screen.getByText(approval.pattern_preview)).toBeVisible();
    const argumentsToggle = screen.getByRole('button', { name: /arguments/i });
    expect(argumentsToggle).toHaveAttribute('aria-expanded', 'false');
    await user.click(argumentsToggle);
    expect(screen.getByText(/<script>alert\(1\)<\/script>/)).toBeVisible();
    expect(container.querySelector('script')).toBeNull();
    expect(approvalApi.previewApprovalScope).not.toHaveBeenCalled();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('labels authoritative risk with an accessible explanation', () => {
    render(<ApprovalReviewPage {...props({ ...approval, risk_level: 'critical' })} />);
    expect(screen.getByText('Critical Risk')).toHaveAccessibleDescription(/server/i);
  });

  it('offers one accent decision without preselecting remembered persistence', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    const { container } = render(<ApprovalReviewPage {...callbacks} />);
    expect(container.querySelectorAll('button[data-variant="primary"]')).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Approve once' })).toHaveAttribute('data-variant', 'primary');
    expect(screen.getByRole('button', { name: 'Deny', exact: true })).toHaveAttribute('data-variant', 'secondary');
    expect(screen.queryByRole('radio')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Approve and remember' }));
    expect(await screen.findByRole('radio', { name: /for this session/i })).not.toBeChecked();
    expect(screen.getByRole('radio', { name: /always allow/i })).not.toBeChecked();
    expect(callbacks.onApprove).not.toHaveBeenCalled();
  });

  it.each(['session', 'permanent'] as const)('requires server preview and explicit confirmation for %s', async (persistence) => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Approve and remember' }));
    await user.click(await screen.findByRole('radio', { name: persistence === 'session' ? /for this session/i : /always allow/i }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeEnabled());
    expect(callbacks.onApprove).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Confirm approval' }));
    expect(callbacks.onApprove).toHaveBeenCalledWith({ persistence, params_pattern: approval.params_pattern });
  });

  it('keeps rejected patterns editable and blocks confirmation until server validation succeeds', async () => {
    const user = userEvent.setup();
    vi.mocked(approvalApi.previewApprovalScope).mockRejectedValue({ status: 422, code: 'invalid_pattern', message: 'Invalid pattern' });
    render(<ApprovalReviewPage {...props()} errorCode="INVALID_PATTERN" />);
    await user.click(screen.getByRole('button', { name: 'Approve and remember' }));
    await user.click(await screen.findByRole('radio', { name: /for this session/i }));
    await waitFor(() => expect(approvalApi.previewApprovalScope).toHaveBeenCalled());
    expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    expect(screen.getByRole('combobox', { name: 'Path match mode' })).toBeEnabled();
  });

  it('resets remembered persistence and custom pattern when a cached route changes ID', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    const { rerender } = render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Approve and remember' }));
    await user.click(await screen.findByRole('radio', { name: /always allow/i }));
    await user.click(screen.getByRole('button', { name: /approval scope/i }));
    await user.click(screen.getByRole('combobox', { name: 'Path match mode' }));
    await user.click(screen.getByRole('option', { name: 'Any value' }));
    rerender(<ApprovalReviewPage {...callbacks} approval={{ ...approval, id: 'approval-2', params_pattern: { path: '/new' } }} />);
    expect(screen.queryByRole('radio')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    expect(callbacks.onApprove).toHaveBeenCalledWith({ persistence: 'once' });
    await user.click(screen.getByRole('button', { name: 'Approve and remember' }));
    await user.click(await screen.findByRole('radio', { name: /for this session/i }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Confirm approval' }));
    expect(callbacks.onApprove).toHaveBeenLastCalledWith({ persistence: 'session', params_pattern: { path: '/new' } });
  });

  it('requires permanent-denial confirmation and returns focus when canceled', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} />);
    const trigger = screen.getByRole('button', { name: /deny permanently/i });
    await user.click(trigger);
    const dialog = await screen.findByRole('dialog', { name: 'Confirm permanent denial' });
    expect(callbacks.onDeny).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(callbacks.onDeny).not.toHaveBeenCalled();
    await user.click(trigger);
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Confirm permanent denial' }));
    expect(callbacks.onDeny).toHaveBeenCalledWith(true);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it.each(['approved', 'denied', 'expired'] as const)('renders %s without decision controls', (status) => {
    const detail = status === 'expired' ? { ...approval, expires_at: '2000-01-01T00:00:00Z' } : { ...approval, status };
    render(<ApprovalReviewPage {...props(detail)} />);
    expect(screen.queryByRole('button', { name: /approve|deny/i })).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: status === 'expired' ? 'Approval Expired' : status === 'approved' ? 'Approved' : 'Denied' })).toBeVisible();
  });

  it.each(['approved', 'denied'] as const)('bounds unbroken names in the %s outcome and expands escaped text by keyboard', async (status) => {
    const user = userEvent.setup();
    const toolName = `${'t'.repeat(320)}<img src=x onerror=alert(1)>`;
    const agentName = `${'a'.repeat(320)}<script>alert(2)</script>`;
    render(<ApprovalReviewPage {...props({ ...approval, status, tool_name: toolName, agent_display_name: agentName })} />);
    const outcome = screen.getByRole('status');
    const expand = within(outcome).getByRole('button', { name: 'Show more' });
    const context = document.getElementById(expand.getAttribute('aria-controls')!);
    expect(context).toHaveClass('line-clamp-2');
    expect(context).toHaveTextContent(`${toolName} for ${agentName}`);
    expect(outcome.querySelector('img, script')).toBeNull();
    await user.tab();
    expect(expand).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(within(outcome).getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true');
    expect(context).not.toHaveClass('line-clamp-2');
    await user.keyboard(' ');
    expect(expand).toHaveAttribute('aria-expanded', 'false');
    expect(context).toHaveClass('line-clamp-2');
  });

  it.each(['approved', 'denied'] as const)('keeps the conflict notice alongside the authoritative %s outcome without offering actions', async (status) => {
    const user = userEvent.setup();
    const callbacks = props();
    const { rerender } = render(<ApprovalReviewPage {...callbacks} />);
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    rerender(<ApprovalReviewPage {...callbacks} approval={{ ...approval, status }} errorCode="ALREADY_ACTIONED" />);
    expect(within(screen.getByRole('alert')).getByRole('heading', { name: 'Already Resolved' })).toBeVisible();
    expect(within(screen.getByRole('status')).getByRole('heading', { name: status === 'approved' ? 'Approved' : 'Denied' })).toBeVisible();
    expect(screen.getByTestId('approval-outcome')).toHaveTextContent(status === 'approved' ? 'Approved' : 'Denied');
    expect(screen.queryByRole('button', { name: /approve|deny/i })).not.toBeInTheDocument();
  });

  it('keeps network failure non-successful and leaves a deliberate decision available', async () => {
    const user = userEvent.setup();
    const callbacks = props();
    render(<ApprovalReviewPage {...callbacks} errorCode="NETWORK_ERROR" errorMessage="Unable to connect" />);
    expect(within(screen.getByRole('alert')).getByRole('heading', { name: 'Connection Error' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Approve once' }));
    expect(callbacks.onApprove).toHaveBeenCalledWith({ persistence: 'once' });
  });
});
