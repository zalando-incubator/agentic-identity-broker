import { AxiosError, CanceledError, isCancel } from 'axios';
import { afterEach, describe, expect, it } from 'vitest';
import { approvalApi } from '@services/api/approvals';
import { apiClient } from '@services/api/client';
import { createQueryClient } from './queryClient';

const adapter = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = adapter; });

describe('security-safe query retries', () => {
  it.each([400, 401, 403, 404, 409, 410, 422, 429])('never replays a %s read', async status => {
    const client = createQueryClient();
    let requests = 0;
    apiClient.defaults.adapter = async config => {
      requests += 1;
      throw new AxiosError('Rejected', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: status === 422 ? 'invalid_pattern' : 'invalid_request', message: 'Rejected' },
        status, statusText: String(status), headers: {},
      });
    };
    const failure: unknown = await client.fetchQuery({
      queryKey: ['principal', 'alice', 'read'],
      queryFn: ({ signal }) => approvalApi.getApproval('approval', { signal }),
      retryDelay: 0,
    }).catch(error => error);
    expect(failure).toHaveProperty('status', status);
    expect(failure).not.toBeInstanceOf(AxiosError);
    expect(requests).toBe(1);
    client.clear();
  });

  it('bounds network retries and never retries a security mutation', async () => {
    const client = createQueryClient();
    let requests = 0;
    apiClient.defaults.adapter = async config => {
      requests += 1;
      throw new AxiosError('offline', 'ERR_NETWORK', config);
    };
    const failure: unknown = await client.fetchQuery({
      queryKey: ['principal', 'alice', 'read'],
      queryFn: ({ signal }) => approvalApi.getApproval('approval', { signal }),
      retryDelay: 0,
    }).catch(error => error);
    expect(failure).toMatchObject({ status: 0, code: 'NETWORK_ERROR' });
    expect(requests).toBe(3);
    const mutation = client.getMutationCache().build(client, {
      mutationFn: () => approvalApi.denyApproval('approval'), retryDelay: 0,
    });
    await expect(mutation.execute(undefined)).rejects.toMatchObject({ status: 0, code: 'NETWORK_ERROR' });
    expect(requests).toBe(4);
    client.clear();
  });

  it('never retries a canceled API read', async () => {
    const client = createQueryClient();
    const canceled = new CanceledError();
    let requests = 0;
    apiClient.defaults.adapter = async () => { requests += 1; throw canceled; };
    const failure: unknown = await client.fetchQuery({
      queryKey: ['principal', 'alice', 'read'],
      queryFn: ({ signal }) => approvalApi.getApproval('approval', { signal }),
      retryDelay: 0,
    }).catch(error => error);
    expect(failure).toBe(canceled);
    expect(isCancel(failure)).toBe(true);
    expect(requests).toBe(1);
    client.clear();
  });
});
