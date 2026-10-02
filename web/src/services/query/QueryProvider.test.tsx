import { StrictMode } from 'react';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { useQuery } from '@tanstack/react-query';
import { AxiosError, CanceledError, type AxiosAdapter } from 'axios';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from '@services/api/client';
import { consentApi } from '@services/api/consent';
import { QueryProvider, usePrincipal } from './QueryProvider';
import { createQueryClient } from './queryClient';
import { queryKeys } from './queryKeys';

const originalAdapter = apiClient.defaults.adapter;
afterEach(() => { cleanup(); apiClient.defaults.adapter = originalAdapter; vi.restoreAllMocks(); });

function PrincipalContent({ onRender }: { onRender?: (principal: string) => void }) {
  const user = usePrincipal();
  onRender?.(user.principal);
  useQuery({ queryKey: queryKeys.delegations(user.principal), queryFn: ({ signal }) => consentApi.getAgentDelegations({ signal }) });
  return <p>{user.displayName}</p>;
}

describe('QueryProvider identity boundary', () => {
  it('does not render private content until identity is authenticated and never persists API data', async () => {
    const identity = Promise.withResolvers<{ principal: string; displayName: string }>();
    vi.spyOn(consentApi, 'getUserInfo').mockReturnValue(identity.promise);
    vi.spyOn(consentApi, 'getAgentDelegations').mockResolvedValue([]);
    const storage = vi.spyOn(Storage.prototype, 'setItem');
    const child = vi.fn();
    const client = createQueryClient();
    render(<QueryProvider client={client} fallback={<p>Loading identity</p>}><PrincipalContent onRender={child} /></QueryProvider>);
    expect(child).not.toHaveBeenCalled();
    expect(screen.getByText('Loading identity')).toBeInTheDocument();
    await act(async () => { identity.resolve({ principal: 'alice', displayName: 'Alice' }); });
    expect(await screen.findByText('Alice')).toBeInTheDocument();
    expect(storage).not.toHaveBeenCalled();
    client.clear();
  });

  it('aborts and removes the previous principal before rendering a different user', async () => {
    vi.spyOn(consentApi, 'getUserInfo')
      .mockResolvedValueOnce({ principal: 'alice', displayName: 'Alice' })
      .mockResolvedValue({ principal: 'bob', displayName: 'Bob' });
    let aliceSignal: AbortSignal | undefined;
    vi.spyOn(consentApi, 'getAgentDelegations').mockImplementation(({ signal } = {}) => {
      aliceSignal ??= signal;
      return Promise.withResolvers<never>().promise;
    });
    const client = createQueryClient();
    const seen: string[] = [];
    render(<QueryProvider client={client}><PrincipalContent onRender={(principal) => {
      seen.push(principal);
      if (principal === 'bob') expect(client.getQueryData(queryKeys.grant('alice', 'agent'))).toBeUndefined();
    }} /></QueryProvider>);
    await screen.findByText('Alice');
    client.setQueryData(queryKeys.grant('alice', 'agent'), { private: true });
    await act(async () => { await client.refetchQueries({ queryKey: ['identity'] }); });
    await screen.findByText('Bob');
    expect(aliceSignal?.aborted).toBe(true);
    expect(client.getQueryCache().findAll({ queryKey: queryKeys.principal('alice') })).toHaveLength(0);
    expect(seen).toContain('bob');
    client.clear();
  });

  it('a transport 401 closes the render gate and cancels all private reads', async () => {
    let pendingSignal: AbortSignal | undefined;
    const adapter: AxiosAdapter = (config) => {
      if (config.url === '/me') return Promise.resolve({ data: { data: { principal: 'alice', displayName: 'Alice' } }, status: 200, statusText: 'OK', headers: {}, config });
      if (config.url === '/lost-auth') return Promise.reject(new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, { data: {}, status: 401, statusText: 'Unauthorized', headers: {}, config }));
      pendingSignal = config.signal as AbortSignal;
      const pending = Promise.withResolvers<never>();
      config.signal?.addEventListener?.('abort', () => pending.reject(new CanceledError()));
      return pending.promise;
    };
    apiClient.defaults.adapter = adapter;
    const client = createQueryClient();
    render(<QueryProvider client={client} errorFallback={() => <p>Sign in again</p>}><PrincipalContent /></QueryProvider>);
    await screen.findByText('Alice');
    client.setQueryData(queryKeys.pending('alice'), [{ id: 'private' }]);
    await act(async () => { await apiClient.get('/lost-auth').catch(() => undefined); });
    await waitFor(() => expect(screen.queryByText('Alice')).not.toBeInTheDocument());
    expect(screen.getByText('Sign in again')).toBeInTheDocument();
    expect(pendingSignal?.aborted).toBe(true);
    expect(client.getQueryCache().findAll({ queryKey: queryKeys.principal('alice') })).toHaveLength(0);
    client.clear();
  });

  it('ignores a departed principal’s delayed mutation 401 but fails closed on the current principal’s 401', async () => {
    let user = { principal: 'alice', displayName: 'Alice' };
    const oldFailure = Promise.withResolvers<never>();
    const oldStarted = Promise.withResolvers<void>();
    let rejectOldRequest!: () => void;
    apiClient.defaults.adapter = (config) => {
      const response = { config, status: 200, statusText: 'OK', headers: {} };
      if (config.url === '/me') return Promise.resolve({ ...response, data: { data: user } });
      if (config.method === 'delete') {
        rejectOldRequest = () => oldFailure.reject(new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, {
          ...response, status: 401, statusText: 'Unauthorized', data: {},
        }));
        oldStarted.resolve();
        return oldFailure.promise;
      }
      if (config.url === '/current-auth-loss') {
        return Promise.reject(new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, {
          ...response, status: 401, statusText: 'Unauthorized', data: {},
        }));
      }
      return Promise.resolve({ ...response, data: { data: [] } });
    };
    const client = createQueryClient();
    render(
      <StrictMode>
        <QueryProvider client={client} errorFallback={() => <p>Sign in again</p>}>
          <PrincipalContent />
        </QueryProvider>
      </StrictMode>,
    );
    await screen.findByText('Alice');
    const departedResult = consentApi.deleteGrant('agent').catch((error: unknown) => error);
    await oldStarted.promise;
    user = { principal: 'bob', displayName: 'Bob' };
    await act(async () => { await client.refetchQueries({ queryKey: ['identity'] }); });
    await screen.findByText('Bob');
    const bobData = [{ id: 'bob-private-approval' }];
    client.setQueryData(queryKeys.pending('bob'), bobData);

    await act(async () => { rejectOldRequest(); await departedResult; });
    expect(await departedResult).toMatchObject({ status: 401, code: 'UNAUTHORIZED', retryable: false });
    expect(screen.getByText('Bob')).toBeInTheDocument();
    expect(screen.queryByText('Sign in again')).not.toBeInTheDocument();
    expect(client.getQueryData(queryKeys.pending('bob'))).toEqual(bobData);

    await act(async () => { await apiClient.get('/current-auth-loss').catch(() => undefined); });
    await screen.findByText('Sign in again');
    expect(screen.queryByText('Bob')).not.toBeInTheDocument();
    expect(client.getQueryCache().findAll({ queryKey: queryKeys.principal('bob') })).toHaveLength(0);
    client.clear();
  });
});
