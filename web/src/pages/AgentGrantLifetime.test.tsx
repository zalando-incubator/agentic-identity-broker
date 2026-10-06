import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Link } from 'react-router-dom';
import { AxiosError } from 'axios';
import { consentApi } from '@services/api/consent';
import { apiClient } from '@services/api/client';
import { AgentDecisionPage } from './AgentDecisionPage';
import { AgentConsolePage } from './AgentConsolePage';
import { detail, grant, renderAgentPage } from './agentConsentTestSupport';
import type { GrantResult, UserInfo } from '../types/consent';

vi.mock('@services/api/consent', () => ({ consentApi: { getUserInfo: vi.fn(), getAgentDetail: vi.fn(), getAgentGrants: vi.fn(), createOrUpdateGrant: vi.fn(), deleteGrant: vi.fn() } }));
const originalAdapter = apiClient.defaults.adapter;
const originalUrl = window.location.href;
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'alice', displayName: 'Alice' });
  vi.mocked(consentApi.getAgentDetail).mockResolvedValue(detail);
  vi.mocked(consentApi.getAgentGrants).mockResolvedValue([grant]);
  window.history.replaceState(null, '', originalUrl);
});
afterEach(() => {
  cleanup();
  apiClient.defaults.adapter = originalAdapter;
  window.history.replaceState(null, '', originalUrl);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe.each(['decision', 'console'] as const)('%s grant continuation lifetime', (context) => {
  it.each(['principal change', 'authentication loss', 'route departure'] as const)('does not follow a held grant response after %s', async (departure) => {
    const response = Promise.withResolvers<GrantResult>();
    vi.mocked(consentApi.createOrUpdateGrant).mockReturnValue(response.promise);
    const page = context === 'decision' ? <AgentDecisionPage /> : <AgentConsolePage />;
    const { client } = renderAgentPage(<><Link to="/agents">Leave agent</Link>{page}</>, context === 'decision' ? '?session_token=authorization' : '');
    expect(await screen.findByTestId('agent-name-heading')).toHaveTextContent(detail.agent.displayName);
    if (context === 'console') {
      await userEvent.click(screen.getByRole('combobox', { name: 'Access lasts' }));
      await userEvent.click(await screen.findByRole('option', { name: 'Until I revoke it' }));
    }
    await userEvent.click(screen.getByRole('button', { name: context === 'decision' ? 'Allow' : 'Save changes', exact: true }));
    await waitFor(() => expect(consentApi.createOrUpdateGrant).toHaveBeenCalledTimes(1));
    const mutation = client.getMutationCache().getAll().find((entry) => entry.state.status === 'pending')!;

    if (departure === 'principal change') {
      vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
      vi.mocked(consentApi.getAgentGrants).mockResolvedValue([{ ...grant, principal: 'bob' }]);
      await act(async () => { await client.refetchQueries({ queryKey: ['identity'] }); });
      await waitFor(() => expect(client.getQueryData<UserInfo>(['identity'])?.principal).toBe('bob'));
      await screen.findByRole('button', { name: context === 'decision' ? 'Allow' : 'Revoke access', exact: true });
    } else if (departure === 'authentication loss') {
      apiClient.defaults.adapter = async (config) => {
        throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, { data: {}, status: 401, statusText: 'Unauthorized', headers: {}, config });
      };
      await act(async () => { await apiClient.get('/lost-auth').catch(() => undefined); });
      await waitFor(() => expect(screen.queryByTestId('agent-name-heading')).not.toBeInTheDocument());
    } else {
      await userEvent.click(screen.getByRole('link', { name: 'Leave agent' }));
      if (context === 'console') await userEvent.click(await screen.findByRole('button', { name: 'Discard changes' }));
      expect(await screen.findByText('Agent list')).toBeVisible();
    }

    const before = window.location.href;
    await act(async () => { response.resolve({ kind: 'redirect', redirectUrl: `${before.split('#')[0]}#departed-grant` }); });
    await waitFor(() => expect(mutation.state.status).not.toBe('pending'));
    expect(window.location.href).toBe(before);
    expect(screen.queryByText('Grant updated successfully.')).not.toBeInTheDocument();
    expect(screen.queryByTestId('consent-outcome')).not.toBeInTheDocument();
    client.clear();
  });
});

async function allowWithDelayedContinuation(reduced = false) {
  vi.stubGlobal('matchMedia', vi.fn((query: string) => ({ matches: reduced && query === '(prefers-reduced-motion: reduce)', addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  const continuation = `${window.location.href.split('#')[0]}#continued`;
  vi.mocked(consentApi.createOrUpdateGrant).mockResolvedValue({ kind: 'redirect', redirectUrl: continuation });
  const { client } = renderAgentPage(<><Link to="/agents">Leave agent</Link><AgentDecisionPage /></>, '?session_token=authorization');
  await screen.findByTestId('agent-name-heading');
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  fireEvent.click(screen.getByRole('button', { name: 'Allow', exact: true }));
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  expect(screen.getByTestId('consent-outcome')).toBeVisible();
  return { client, continuation };
}

describe('confirmed consent continuation', () => {
  it('shows success until the animation completes, then follows the server continuation', async () => {
    const { client, continuation } = await allowWithDelayedContinuation();
    const before = window.location.href;
    await act(async () => { await vi.advanceTimersByTimeAsync(319); });
    expect(window.location.href).toBe(before);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(window.location.href).toBe(continuation);
    client.clear();
  });

  it.each(['principal change', 'authentication loss', 'route departure'] as const)('does not follow a confirmed continuation after %s during success feedback', async departure => {
    const { client } = await allowWithDelayedContinuation();
    if (departure === 'principal change') {
      vi.mocked(consentApi.getUserInfo).mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
      vi.mocked(consentApi.getAgentGrants).mockResolvedValue([{ ...grant, principal: 'bob' }]);
      await act(async () => { await client.refetchQueries({ queryKey: ['identity'] }); await vi.advanceTimersByTimeAsync(0); });
      expect(client.getQueryData<UserInfo>(['identity'])?.principal).toBe('bob');
    } else if (departure === 'authentication loss') {
      apiClient.defaults.adapter = async config => { throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, { data: {}, status: 401, statusText: 'Unauthorized', headers: {}, config }); };
      await act(async () => { await apiClient.get('/lost-auth').catch(() => undefined); await vi.advanceTimersByTimeAsync(0); });
      expect(screen.queryByTestId('agent-name-heading')).not.toBeInTheDocument();
    } else {
      fireEvent.click(screen.getByRole('link', { name: 'Leave agent' }));
      expect(screen.getByText('Agent list')).toBeVisible();
    }
    const before = window.location.href;
    await act(async () => { await vi.advanceTimersByTimeAsync(320); });
    expect(window.location.href).toBe(before);
    client.clear();
  });

  it('follows the continuation without an animation wait under reduced motion', async () => {
    const { client, continuation } = await allowWithDelayedContinuation(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(window.location.href).toBe(continuation);
    client.clear();
  });
});
