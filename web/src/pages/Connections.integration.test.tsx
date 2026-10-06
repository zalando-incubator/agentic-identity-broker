import { describe, expect, it } from 'vitest';
import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { SessionSummary } from '@services/api/sessions';
import { renderApplication, type HttpReply } from './applicationIntegrationTestSupport';

const mail: SessionSummary = {
  id: 'mail-session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer',
  scope: ['read', 'send'], initiated_at: '2026-01-01T00:00:00Z', is_expired: false,
  access_token_expired: false, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};
const affectedPath = '/third-party/mail/session/affected-agents';
const affected = [{ agent_id: 'agent', display_name: 'Research Agent' }];

describe('connections through real session clients and confirmation dialogs', () => {
  it('waits for affected agents, warns about lost access, cancels safely and refetches before confirmed disconnect', async () => {
    const user = userEvent.setup();
    const dependencies = Promise.withResolvers<HttpReply>();
    const deletion = Promise.withResolvers<HttpReply>();
    let sessions = [mail];
    let dependencyReads = 0;
    const { requests } = renderApplication('/connections', config => {
      if (config.url === '/third-party/sessions') return { body: { data: { sessions } } };
      if (config.url === affectedPath) return ++dependencyReads === 1 ? dependencies.promise : { body: { data: { affected_agents: affected } } };
      if (config.method === 'delete' && config.url === '/third-party/mail/session') return deletion.promise;
    });

    const card = await screen.findByTestId('connection-card');
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Connected');
    await user.click(within(card).getByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('dialog', { name: 'Disconnect service' });
    expect(dialog).toHaveTextContent('Disconnect Mail from this broker?');
    expect(dialog).toHaveTextContent(/does not revoke provider-side tokens/i);
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeDisabled();
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(0);
    await act(async () => dependencies.resolve({ body: { data: { affected_agents: affected } } }));
    expect(await within(dialog).findByText('Research Agent')).toBeVisible();
    expect(dialog).toHaveTextContent('These agents lose access through this connection:');
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeEnabled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(0);
    expect(card).toBeVisible();

    await user.click(within(card).getByRole('button', { name: 'Disconnect' }));
    const confirmation = screen.getByRole('dialog', { name: 'Disconnect service' });
    await within(confirmation).findByText('Research Agent');
    expect(dependencyReads).toBe(2);
    expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'get', url: affectedPath, signal: expect.any(AbortSignal) }));
    await user.click(within(confirmation).getByRole('button', { name: 'Disconnect' }));
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'delete', url: '/third-party/mail/session' })));
    expect(card).toBeVisible();
    expect(within(card).getByRole('button', { name: 'Disconnect' })).toBeDisabled();
    expect(screen.queryByText('Connection disconnected.')).not.toBeInTheDocument();
    sessions = [];
    await act(async () => deletion.resolve({ status: 204, body: undefined }));
    expect(await screen.findByText('Connection disconnected.')).toBeVisible();
    await waitFor(() => expect(screen.queryByTestId('connection-card')).not.toBeInTheDocument());
    expect(await screen.findByRole('heading', { name: 'No connections yet' })).toBeVisible();
  });

  it('fails closed on an affected-agents read error and retains the connection when DELETE is rejected', async () => {
    const user = userEvent.setup();
    let reads = 0;
    const { requests } = renderApplication('/connections', config => {
      if (config.url === '/third-party/sessions') return { body: { data: { sessions: [mail] } } };
      if (config.url === affectedPath) return ++reads === 1
        ? { status: 503, body: { message: 'Dependency lookup unavailable' } }
        : { body: { data: { affected_agents: affected } } };
      if (config.method === 'delete' && config.url === '/third-party/mail/session') return { status: 503, body: { message: 'Secret token unavailable' } };
    });

    await user.click(await screen.findByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('dialog', { name: 'Disconnect service' });
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Could not load connection details. Please try again.');
    expect(within(dialog).getByRole('button', { name: 'Disconnect' })).toBeDisabled();
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(0);
    await user.click(within(dialog).getByRole('button', { name: 'Try again' }));
    expect(await within(dialog).findByText('Research Agent')).toBeVisible();
    expect(reads).toBe(2);
    await user.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    expect(await screen.findByText('Could not disconnect this connection. Please try again.')).toBeVisible();
    const card = screen.getByTestId('connection-card');
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Connected');
    expect(within(card).getByRole('button', { name: 'Disconnect' })).toBeEnabled();
    expect(screen.queryByText('Secret token unavailable')).not.toBeInTheDocument();
    expect(screen.queryByText('Connection disconnected.')).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(1);
  });

  it('replaces expired access state and scopes only with the accepted refresh response', async () => {
    const user = userEvent.setup();
    const refresh = Promise.withResolvers<HttpReply>();
    const { requests } = renderApplication('/connections', config => {
      if (config.url === '/third-party/sessions') return { body: { data: { sessions: [{ ...mail, access_token_expired: true }] } } };
      if (config.method === 'post' && config.url === '/third-party/mail/session/refresh') return refresh.promise;
    });
    const card = await screen.findByTestId('connection-card');
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Needs sign-in');
    await user.click(within(card).getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'post', url: '/third-party/mail/session/refresh' })));
    expect(within(card).getByTestId('connection-state')).toHaveTextContent('Needs sign-in');
    expect(within(card).getByRole('button', { name: 'Refresh' })).toBeDisabled();
    expect(screen.queryByText('Connection refreshed.')).not.toBeInTheDocument();
    await act(async () => refresh.resolve({ body: { data: { ...mail, scope: ['read', 'send', 'write'] } } }));
    expect(await screen.findByText('Connection refreshed.')).toBeVisible();
    await waitFor(() => expect(screen.getByTestId('connection-state')).toHaveTextContent('Connected'));
    const refreshedCard = screen.getByTestId('connection-card');
    expect(within(refreshedCard).getByRole('button', { name: '3 scopes' })).toBeVisible();
    expect(within(refreshedCard).queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
    expect(requests.mock.calls.filter(([config]) => config.method === 'post')).toHaveLength(1);
  });
});
