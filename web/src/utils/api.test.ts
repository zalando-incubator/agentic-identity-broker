import { afterEach, describe, it, expect } from 'vitest';
import { AxiosError, CanceledError, isCancel } from 'axios';
import { apiClient } from '@services/api/client';
import { extractApiError } from './api';

const adapter = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = adapter; });

describe('extractApiError', () => {
  it('shows a backend message after the transport normalizes a generic 4xx', async () => {
    apiClient.defaults.adapter = async (config) => {
      throw new AxiosError('HTTP failure', 'ERR_BAD_REQUEST', config, undefined, {
        config, data: { error: 'invalid_request', message: 'Selected scope is not permitted' },
        status: 400, statusText: 'Bad Request', headers: {},
      });
    };
    const failure: unknown = await apiClient.get('/third-party/sessions').catch(error => error);
    expect(extractApiError(failure, 'fallback')).toBe('Selected scope is not permitted');
  });

  it('uses the page fallback when a request is canceled', async () => {
    apiClient.defaults.adapter = async () => { throw new CanceledError(); };
    const failure: unknown = await apiClient.get('/third-party/sessions').catch(error => error);
    expect(isCancel(failure)).toBe(true);
    expect(extractApiError(failure, 'fallback')).toBe('fallback');
  });

  it('returns err.message for plain Error', () => {
    const err = new Error('something broke');
    expect(extractApiError(err, 'fallback')).toBe('something broke');
  });

  it('returns fallback for unrecognized value', () => {
    expect(extractApiError('not an error', 'fallback')).toBe('fallback');
    expect(extractApiError(null, 'fallback')).toBe('fallback');
    expect(extractApiError(42, 'fallback')).toBe('fallback');
  });
});
