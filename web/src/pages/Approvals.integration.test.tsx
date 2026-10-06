import { describe, expect, it } from 'vitest';
import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ToolApprovalDetail } from '@app-types/approval';
import { renderApplication, type HttpReply } from './applicationIntegrationTestSupport';

const approval: ToolApprovalDetail = {
  id: 'request-1', principal: 'alice', agent_id: 'agent', agent_display_name: 'Research Agent',
  tool_name: 'read_file', arguments: { path: '/docs/report' }, tool_pattern: 'read_file',
  params_pattern: { path: '/docs/report' }, pattern_preview: 'read_file(path=/docs/report)',
  status: 'pending', approval_url: '/approvals/request-1', created_at: '2026-01-01T00:00:00Z',
  expires_at: '2099-01-01T00:00:00Z',
};
const detailPath = '/approvals/request-1';
const decidedAt = '2026-10-05T10:00:00Z';

describe('approval routes with real review actions and HTTP state', () => {
  it.each(['approve', 'deny'] as const)('records a standalone %s only after the server accepts the chosen action', async action => {
    const user = userEvent.setup();
    const decision = Promise.withResolvers<HttpReply>();
    const { requests } = renderApplication(detailPath, config => {
      if (config.method === 'get' && config.url === detailPath) return { body: { data: approval } };
      if (config.method === 'post' && config.url === `${detailPath}/${action}`) return decision.promise;
    });
    const approve = await screen.findByRole('button', { name: 'Approve once' });
    const deny = screen.getByRole('button', { name: 'Deny' });
    expect(screen.getByTestId('approval-acting-user')).toHaveTextContent('alice');
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(0);
    await user.click(action === 'approve' ? approve : deny);
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'post', url: `${detailPath}/${action}` })));
    expect(approve).toBeDisabled();
    expect(deny).toBeDisabled();
    expect(action === 'approve' ? approve : deny).toHaveAttribute('aria-busy', 'true');
    expect(screen.queryByTestId('approval-outcome')).not.toBeInTheDocument();
    const posted = requests.mock.calls.find(([config]) => config.method === 'post')![0];
    expect(JSON.parse(posted.data)).toEqual(action === 'approve' ? { persistence: 'once' } : {});

    await act(async () => decision.resolve({ body: { data: action === 'approve'
      ? { id: approval.id, status: 'approved', persistence: 'once', approved_at: decidedAt }
      : { id: approval.id, status: 'denied', denied_at: decidedAt } } }));
    const outcome = await screen.findByTestId('approval-outcome');
    expect(outcome).toHaveTextContent(action === 'approve' ? 'Approved' : 'Denied');
    expect(outcome).toHaveTextContent('This request is complete and is shown read-only.');
    expect(screen.queryByRole('button', { name: 'Approve once' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(1);
  });

  it('previews and confirms a permanent inbox decision, then revokes it from the real remembered route', async () => {
    const user = userEvent.setup();
    const preview = Promise.withResolvers<HttpReply>();
    const decision = Promise.withResolvers<HttpReply>();
    const revocation = Promise.withResolvers<HttpReply>();
    let pending = [approval];
    let remembered: ToolApprovalDetail[] = [];
    const { requests } = renderApplication('/approvals', config => {
      if (config.url === '/approvals/pending') return { body: { data: pending } };
      if (config.url === '/approvals/permanent') return { body: { data: remembered } };
      if (config.method === 'post' && config.url === `${detailPath}/scope-preview`) return preview.promise;
      if (config.method === 'post' && config.url === `${detailPath}/approve`) return decision.promise;
      if (config.method === 'post' && config.url === `${detailPath}/revoke`) return revocation.promise;
    });
    const list = await screen.findByTestId('pending-approvals');
    expect(within(list).getByTestId('pending-approval-row')).toHaveTextContent('read_file');
    await user.click(screen.getByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Always…' }));
    expect(screen.queryByRole('button', { name: 'Confirm approval' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: 'Always approve' }));
    const confirm = screen.getByRole('button', { name: 'Confirm approval' });
    expect(confirm).toBeDisabled();
    expect(screen.getByText('This grants permanent access. You can revoke it from Approvals.')).toBeVisible();
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'post', url: `${detailPath}/scope-preview` })));
    expect(requests.mock.calls.filter(([config]) => config.url === `${detailPath}/approve`)).toHaveLength(0);
    const previewRequest = requests.mock.calls.find(([config]) => config.url === `${detailPath}/scope-preview`)![0];
    expect(JSON.parse(previewRequest.data)).toEqual({ params_pattern: approval.params_pattern });
    expect(previewRequest.signal).toBeInstanceOf(AbortSignal);
    await act(async () => preview.resolve({ body: { data: { tool_pattern: approval.tool_pattern, params_pattern: approval.params_pattern, preview: approval.pattern_preview } } }));
    await waitFor(() => expect(confirm).toBeEnabled());
    await user.click(confirm);
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'post', url: `${detailPath}/approve` })));
    const posted = requests.mock.calls.find(([config]) => config.url === `${detailPath}/approve`)![0];
    expect(JSON.parse(posted.data)).toEqual({ persistence: 'permanent', params_pattern: approval.params_pattern });
    expect(screen.getByTestId('pending-approvals')).toHaveTextContent('read_file');
    expect(screen.queryByText("You're all caught up")).not.toBeInTheDocument();

    pending = [];
    remembered = [{ ...approval, status: 'approved', persistence: 'permanent', approved_at: decidedAt }];
    await act(async () => decision.resolve({ body: { data: { id: approval.id, status: 'approved', persistence: 'permanent', approved_at: decidedAt } } }));
    expect(await screen.findByText("You're all caught up")).toBeVisible();
    await user.click(screen.getByRole('tab', { name: /^Remembered/ }));
    await waitFor(() => expect(window.location.pathname).toBe('/approvals/remembered'));
    const row = await screen.findByTestId('standing-decision-row');
    expect(row).toHaveTextContent('read_file');
    expect(row).toHaveTextContent('Always allowed');
    await user.click(within(row).getByRole('button', { name: 'Revoke' }));
    const revokeDialog = await screen.findByRole('dialog', { name: 'Revoke remembered decision?' });
    expect(revokeDialog).toHaveTextContent('read_file');
    await user.click(within(revokeDialog).getByRole('button', { name: 'Cancel' }));
    expect(requests.mock.calls.filter(([config]) => config.url === `${detailPath}/revoke`)).toHaveLength(0);
    await user.click(within(row).getByRole('button', { name: 'Revoke' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke decision' }));
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'post', url: `${detailPath}/revoke` })));
    expect(row).toBeVisible();
    remembered = [];
    await act(async () => revocation.resolve({ body: { data: { id: approval.id, status: 'denied', denied_at: decidedAt } } }));
    expect(await screen.findByText('No remembered decisions')).toBeVisible();
    await waitFor(() => expect(screen.queryByTestId('standing-decision-row')).not.toBeInTheDocument());
  });

  it('keeps a remembered approval unsubmitted when the broker rejects its scope preview', async () => {
    const user = userEvent.setup();
    const { requests } = renderApplication(detailPath, config => {
      if (config.method === 'get' && config.url === detailPath) return { body: { data: approval } };
      if (config.method === 'post' && config.url === `${detailPath}/scope-preview`) return { status: 422, body: { error: 'invalid_pattern', message: 'Invalid pattern' } };
    });
    await user.click(await screen.findByRole('button', { name: 'Approve options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'For this session' }));
    await user.click(screen.getByRole('button', { name: 'Approve for this session' }));
    expect(await screen.findByText('The broker could not validate this scope. Adjust the scope or try the preview again.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Confirm approval' })).toBeDisabled();
    expect(requests.mock.calls.filter(([config]) => config.url === `${detailPath}/approve`)).toHaveLength(0);
    expect(screen.queryByTestId('approval-outcome')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Back' }));
    expect(screen.getByRole('button', { name: 'Approve for this session' })).toBeEnabled();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(1);
  });

  it('refetches an already-resolved decision after a 409 and removes all decision controls', async () => {
    const user = userEvent.setup();
    let current = approval;
    let detailReads = 0;
    const { requests } = renderApplication(detailPath, config => {
      if (config.method === 'get' && config.url === detailPath) {
        detailReads += 1;
        return { body: { data: current } };
      }
      if (config.method === 'post' && config.url === `${detailPath}/approve`) {
        current = { ...approval, status: 'denied', denied_at: decidedAt };
        return { status: 409, body: { message: 'This request was denied in another tab.' } };
      }
    });
    await user.click(await screen.findByRole('button', { name: 'Approve once' }));
    expect(await screen.findByText('Already resolved')).toBeVisible();
    const outcome = await screen.findByTestId('approval-outcome');
    expect(outcome).toHaveTextContent('Denied');
    expect(outcome).toHaveTextContent('This request is complete and is shown read-only.');
    expect(detailReads).toBe(2);
    expect(screen.queryByRole('button', { name: 'Approve once' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(1);
  });

  it('shows an expired request as a terminal state without issuing a decision', async () => {
    const { requests } = renderApplication(detailPath, config => {
      if (config.url === detailPath) return { status: 410, body: { message: 'The request expired.' } };
    });
    expect(await screen.findByText('Approval expired')).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Approve once' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(0);
  });
});
