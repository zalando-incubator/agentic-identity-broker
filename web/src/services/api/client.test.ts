import { AxiosError, CanceledError, isCancel } from 'axios';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { approvalApi } from './approvals';
import * as clientModule from './client';

const { apiClient, isApiError } = clientModule;
const adapter = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = adapter; vi.restoreAllMocks(); });

describe('API transport security errors', () => {
  it('notifies auth-loss subscribers on 401 and supports unsubscribing', async () => {
    const lost = vi.fn();
    const removed = vi.fn();
    const generation = clientModule.getAuthGeneration();
    const unsubscribe = clientModule.subscribeAuthLoss(lost);
    const remove = clientModule.subscribeAuthLoss(removed);
    remove();
    apiClient.defaults.adapter = async (config) => { throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, { config, data: {}, status: 401, statusText: 'Unauthorized', headers: {} }); };
    try {
      await expect(apiClient.get('/me')).rejects.toMatchObject({ status: 401, code: 'UNAUTHORIZED', retryable: false });
      expect(lost).toHaveBeenCalledTimes(1);
      expect(removed).not.toHaveBeenCalled();
      expect(clientModule.getAuthGeneration()).toBe(generation + 1);
    } finally { unsubscribe(); }
  });

  it('preserves cancellation instead of labelling it a retryable network error', async () => {
    const canceled = new CanceledError();
    apiClient.defaults.adapter = async () => { throw canceled; };
    const failure: unknown = await apiClient.get('/me').catch(error => error);
    expect(failure).toBe(canceled);
    expect(isCancel(failure)).toBe(true);
  });

  it('normalizes the approval API invalid-pattern response into one usable error', async () => {
    apiClient.defaults.adapter = async (config) => {
      throw new AxiosError('Invalid', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: 'invalid_pattern', message: 'params_pattern is not allowed for once persistence' },
        status: 422, statusText: 'Unprocessable Entity', headers: {},
      });
    };
    const error: unknown = await approvalApi.approveApproval('one', { persistence: 'once', params_pattern: {} }).catch(failure => failure);
    expect(error).toEqual({ status: 422, code: 'invalid_pattern', message: 'params_pattern is not allowed for once persistence' });
    expect(isApiError(error)).toBe(true);
  });

  it.each([
    [403, 'FORBIDDEN'], [404, 'NOT_FOUND'], [409, 'CONFLICT'],
    [410, 'GONE'], [502, 'SERVER_ERROR'],
  ] as const)('keeps status %i classification without exposing raw server errors', async (status, code) => {
    apiClient.defaults.adapter = async (config) => {
      throw new AxiosError('HTTP failure', 'ERR_BAD_RESPONSE', config, undefined, {
        config, data: { error: 'internal_error', message: 'raw server message' },
        status, statusText: String(status), headers: {},
      });
    };
    const failure: unknown = await apiClient.get('/approvals/one').catch(error => error);
    expect(failure).toMatchObject({ status, code });
    if (status === 409 || status === 410) expect(failure).toHaveProperty('message', 'raw server message');
    else expect(failure).not.toHaveProperty('message', 'raw server message');
  });

  it('keeps a machine code and message when an optional server message is absent', async () => {
    apiClient.defaults.adapter = async (config) => {
      throw new AxiosError('HTTP failure', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: 'invalid_request' }, status: 400, statusText: 'Bad Request', headers: {},
      });
    };
    const failure: unknown = await apiClient.get('/approvals/one').catch(error => error);
    expect(failure).toMatchObject({ status: 400, code: 'invalid_request', message: expect.any(String) });
    expect(isApiError(failure)).toBe(true);
  });

  it('identifies a transport failure separately from a canceled request', async () => {
    apiClient.defaults.adapter = async config => { throw new AxiosError('offline', 'ERR_NETWORK', config); };
    const failure: unknown = await apiClient.get('/me').catch(error => error);
    expect(failure).toMatchObject({ status: 0, code: 'NETWORK_ERROR', retryable: true });
    expect(isCancel(failure)).toBe(false);
  });
});

describe('isApiError', () => {
  it('distinguishes normalized API failures from unknown thrown values', () => {
    expect(isApiError({ status: 404, code: 'NOT_FOUND', message: 'not found' })).toBe(true);
    for (const value of [null, 'string', { status: 404 }]) expect(isApiError(value)).toBe(false);
  });
});
