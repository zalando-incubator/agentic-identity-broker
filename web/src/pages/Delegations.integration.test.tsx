import { describe, expect, it } from 'vitest';
import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { AgentDelegation } from '@app-types/consent';
import { agentDetail, grant, renderApplication, type HttpReply } from './applicationIntegrationTestSupport';

const research: AgentDelegation = { agentId: 'agent', displayName: 'Research Agent', activeGrantCount: 1, lastModifiedAt: grant.updated_at, expiresAt: grant.valid_until };
const helper: AgentDelegation = { agentId: 'helper', displayName: 'Mail Helper', activeGrantCount: 1, lastModifiedAt: grant.updated_at };

describe('delegation list and detail route integration', () => {
  it('opens a real grant detail, cancels revoke, then returns to the updated list only after DELETE succeeds', async () => {
    const user = userEvent.setup();
    const deletion = Promise.withResolvers<HttpReply>();
    let rows = [research, helper];
    const { requests } = renderApplication('/agents', config => {
      if (config.url === '/consent/agents') return { body: { data: rows } };
      if (config.url === '/consent/agents/agent') return { body: { data: agentDetail } };
      if (config.method === 'get' && config.url === '/consent/agents/agent/grants') return { body: { data: [grant] } };
      if (config.url === '/third-party/sessions') return { body: { data: { sessions: [] } } };
      if (config.method === 'delete' && config.url === '/consent/agents/agent/grants') return deletion.promise;
    });

    const link = await screen.findByRole('link', { name: 'Research Agent' });
    await user.click(link);
    expect(await screen.findByRole('heading', { name: 'Research Agent' })).toBeVisible();
    expect(window.location.pathname).toBe('/agents/agent');
    expect(screen.getByRole('checkbox', { name: 'Existing access' })).toBeChecked();
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(0);

    await user.click(screen.getByRole('button', { name: 'Revoke access' }));
    const dialog = await screen.findByRole('dialog', { name: 'Revoke access' });
    expect(dialog).toHaveTextContent('Research Agent');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(0);
    expect(window.location.pathname).toBe('/agents/agent');

    await user.click(screen.getByRole('button', { name: 'Revoke access' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke' }));
    await waitFor(() => expect(requests).toHaveBeenCalledWith(expect.objectContaining({ method: 'delete', url: '/consent/agents/agent/grants' })));
    expect(window.location.pathname).toBe('/agents/agent');
    expect(screen.queryByText('Access revoked.')).not.toBeInTheDocument();
    rows = [helper];
    await act(async () => deletion.resolve({ status: 204, body: undefined }));
    await waitFor(() => expect(window.location.pathname).toBe('/agents'));
    expect(await screen.findByRole('link', { name: 'Mail Helper' })).toBeVisible();
    expect(screen.queryByRole('link', { name: 'Research Agent' })).not.toBeInTheDocument();
    expect(screen.getAllByTestId('agent-entity')).toHaveLength(1);
    expect(requests.mock.calls.filter(([config]) => config.url === '/consent/agents').length).toBeGreaterThan(1);
  });

  it('keeps the rejected row and other agent available without automatic mutation retries', async () => {
    const user = userEvent.setup();
    const { requests } = renderApplication('/agents', config => {
      if (config.url === '/consent/agents') return { body: { data: [research, helper] } };
      if (config.method === 'delete' && config.url === '/consent/agents/agent/grants') return { status: 503, body: { message: 'Storage unavailable' } };
    });
    const entity = (await screen.findByRole('link', { name: 'Research Agent' })).closest<HTMLElement>('[data-testid="agent-entity"]')!;
    await user.click(within(entity).getByRole('button', { name: 'Revoke' }));
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(0);
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke' }));
    expect(await screen.findByText('Could not revoke access. Try again.')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Research Agent' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Mail Helper' })).toBeVisible();
    expect(within(entity).getByRole('button', { name: 'Revoke' })).toBeEnabled();
    expect(requests.mock.calls.filter(([config]) => config.method === 'delete')).toHaveLength(1);
    expect(screen.queryByText('Access revoked.')).not.toBeInTheDocument();
  });

  it('retries a failed list read without presenting a false empty account', async () => {
    const user = userEvent.setup();
    let reads = 0;
    const { requests } = renderApplication('/agents', config => {
      if (config.url === '/consent/agents') return ++reads === 1
        ? { status: 503, body: { message: 'Unavailable' } }
        : { body: { data: [research] } };
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load agents.');
    expect(screen.queryByTestId('delegations-empty-state')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('link', { name: 'Research Agent' })).toBeVisible();
    expect(reads).toBe(2);
    expect(requests.mock.calls.filter(([config]) => config.method !== 'get')).toHaveLength(0);
  });
});
