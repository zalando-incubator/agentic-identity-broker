import { AxiosError, CanceledError } from 'axios';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as clientModule from './client';

const { apiClient, isApiError } = clientModule;
const adapter = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = adapter; vi.restoreAllMocks(); });

describe('API transport security errors', () => {
  it('notifies auth-loss subscribers on 401 and supports unsubscribing', async () => {
    const lost = vi.fn();
    const removed = vi.fn();
    const unsubscribe = clientModule.subscribeAuthLoss?.(lost);
    const remove = clientModule.subscribeAuthLoss?.(removed);
    remove?.();
    apiClient.defaults.adapter = async (config) => { throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, { config, data: {}, status: 401, statusText: 'Unauthorized', headers: {} }); };
    try {
      await expect(apiClient.get('/me')).rejects.toMatchObject({ status: 401, code: 'UNAUTHORIZED', retryable: false });
      expect(lost).toHaveBeenCalledTimes(1);
      expect(removed).not.toHaveBeenCalled();
    } finally { unsubscribe?.(); }
  });

  it('preserves cancellation instead of labelling it a retryable network error', async () => {
    apiClient.defaults.adapter = async () => { throw new CanceledError(); };
    await expect(apiClient.get('/me')).rejects.toHaveProperty('code', 'ERR_CANCELED');
  });

  it('preserves the HTTP status when a structured 4xx body omits it', async () => {
    apiClient.defaults.adapter = async (config) => { throw new AxiosError('Invalid', 'ERR_BAD_REQUEST', config, undefined, { config, data: { code: 'INVALID_PATTERN', message: 'Invalid scope' }, status: 422, statusText: 'Unprocessable Entity', headers: {} }); };
    await expect(apiClient.post('/approvals/id/scope-preview', {})).rejects.toMatchObject({ status: 422, code: 'INVALID_PATTERN', message: 'Invalid scope' });
  });
});

describe('isApiError', () => {
  it('distinguishes normalized API failures from unknown thrown values', () => {
    expect(isApiError({ status: 404, code: 'NOT_FOUND', message: 'not found' })).toBe(true);
    for (const value of [null, 'string', { status: 404 }]) expect(isApiError(value)).toBe(false);
  });
});
