import { CanceledError } from 'axios';
import { afterEach, describe, expect, it } from 'vitest';
import { apiClient } from './client';
import { approvalApi } from './approvals';

const adapter = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = adapter; });

describe('approval request cancellation', () => {
  it.each([
    ['pending', (signal: AbortSignal) => approvalApi.listPendingApprovals({ signal }), []],
    ['standing', (signal: AbortSignal) => approvalApi.listPermanentApprovals({ signal }), []],
    ['detail', (signal: AbortSignal) => approvalApi.getApproval('approval', { signal }), { id: 'approval' }],
    ['scope preview', (signal: AbortSignal) => approvalApi.previewApprovalScope('approval', {}, { signal }), { tool_pattern: 'read', params_pattern: {}, preview: 'read()' }],
  ] as const)('cancels an in-flight %s read without making it retryable', async (_, read, data) => {
    apiClient.defaults.adapter = (config) => {
      if (!config.signal) return Promise.resolve({ config, data: { data }, status: 200, statusText: 'OK', headers: {} });
      const pending = Promise.withResolvers<never>();
      config.signal.addEventListener?.('abort', () => pending.reject(new CanceledError()));
      return pending.promise;
    };
    const controller = new AbortController();
    const result = read(controller.signal);
    controller.abort();
    await expect(result).rejects.toHaveProperty('code', 'ERR_CANCELED');
  });
});
