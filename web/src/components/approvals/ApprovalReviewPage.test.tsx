import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApprovalReviewPage } from './ApprovalReviewPage';

const approval = {
  id: 'approval-1',
  principal: 'user@example.com',
  agent_id: 'agent-1',
  agent_display_name: 'Research Assistant',
  tool_name: 'read_file',
  arguments: { path: '/tmp/example' },
  tool_pattern: 'read_file',
  params_pattern: { path: '/tmp/example' },
  status: 'pending' as const,
  approval_url: 'https://broker.example.com/approvals/approval-1',
  created_at: '2026-03-29T00:00:00Z',
  expires_at: '2026-03-29T00:10:00Z',
};

describe('ApprovalReviewPage', () => {
  it('keeps actions available after a network error', async () => {
    const user = userEvent.setup();
    const onApprove = vi.fn().mockResolvedValue(undefined);
    const onDeny = vi.fn().mockResolvedValue(undefined);
    const onRetry = vi.fn();

    render(
      <ApprovalReviewPage
        approval={approval}
        submitting={false}
        errorCode="NETWORK_ERROR"
        errorMessage="Unable to connect"
        approveResult={null}
        denyResult={null}
        onApprove={onApprove}
        onDeny={onDeny}
        onRetry={onRetry}
      />,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('Connection Error');
    expect(screen.getByRole('button', { name: /^approve$/i })).toBeEnabled();
    expect(screen.getByRole('button', { name: /^deny$/i })).toBeEnabled();

    await user.click(screen.getByRole('button', { name: /^approve$/i }));
    expect(onApprove).toHaveBeenCalledWith({ persistence: 'once' });

    await user.click(screen.getByRole('button', { name: /try again/i }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it('keeps the scope editable after the server rejects the pattern', async () => {
    const user = userEvent.setup();
    const onApprove = vi.fn().mockResolvedValue(undefined);

    render(
      <ApprovalReviewPage
        approval={approval}
        submitting={false}
        errorCode="INVALID_PATTERN"
        errorMessage={null}
        approveResult={null}
        denyResult={null}
        onApprove={onApprove}
        onDeny={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('Scope Not Accepted');

    await user.click(screen.getByRole('radio', { name: /for this session/i }));
    expect(screen.getByRole('button', { name: /approval scope/i })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /^approve$/i }));
    expect(onApprove).toHaveBeenCalledWith({
      persistence: 'session',
      tool_pattern: 'read_file',
      params_pattern: { path: '/tmp/example' },
    });
  });

  it('approves permanently after selecting Always allow', async () => {
    const user = userEvent.setup();
    const onApprove = vi.fn().mockResolvedValue(undefined);

    render(
      <ApprovalReviewPage
        approval={approval}
        submitting={false}
        errorCode={null}
        errorMessage={null}
        approveResult={null}
        denyResult={null}
        onApprove={onApprove}
        onDeny={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('radio', { name: /always allow/i }));
    await user.click(screen.getByRole('button', { name: /^approve$/i }));

    expect(onApprove).toHaveBeenCalledWith({ persistence: 'permanent', tool_pattern: 'read_file', params_pattern: { path: '/tmp/example' } });
  });

  it('shows a read-only recorded decision for an already approved once-only request', () => {
    render(
      <ApprovalReviewPage
        approval={{
          ...approval,
          status: 'approved',
          persistence: 'once',
          approved_at: '2026-03-29T00:01:00Z',
        }}
        submitting={false}
        errorCode={null}
        errorMessage={null}
        approveResult={null}
        denyResult={null}
        onApprove={vi.fn()}
        onDeny={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Approved' })).toBeVisible();
    expect(screen.getByText('Decision recorded')).toBeVisible();
    expect(
      screen.getByText('This request is complete and is shown read-only.'),
    ).toBeVisible();
    expect(screen.getByText('You can safely close this page.')).toBeVisible();
    expect(
      screen.queryByRole('button', { name: /^approve$/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /^deny$/i }),
    ).not.toBeInTheDocument();
  });

  it('shows an immutable historical decision for an already denied request', () => {
    render(
      <ApprovalReviewPage
        approval={{
          ...approval,
          status: 'denied',
          denied_at: '2026-03-29T00:01:00Z',
        }}
        submitting={false}
        errorCode={null}
        errorMessage={null}
        approveResult={null}
        denyResult={null}
        onApprove={vi.fn()}
        onDeny={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Denied' })).toBeVisible();
    expect(screen.getByText('Decision recorded')).toBeVisible();
    expect(
      screen.getByText('This request is complete and is shown read-only.'),
    ).toBeVisible();
    expect(
      screen.queryByRole('button', { name: /^approve$/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /^deny$/i }),
    ).not.toBeInTheDocument();
  });
});
