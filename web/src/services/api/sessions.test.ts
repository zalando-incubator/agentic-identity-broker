import { CanceledError } from 'axios';
import { afterEach, describe, expect, it } from 'vitest';
import { apiClient } from './client';
import { sessionsApi, type SessionDetail, type SessionSummary } from './sessions';

const adapter = apiClient.defaults.adapter;
const summary: SessionSummary = {
  id: 'alice-session', service_id: 'service', service_display_name: 'Service', token_type: 'Bearer',
  scope: ['read'], initiated_at: '2026-01-01T00:00:00Z', is_expired: false,
  access_token_expired: false, has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};
const detail: SessionDetail = {
  session: {
    id: 'alice-session', principal: 'alice', service_id: 'service', token_type: 'Bearer',
    scope: ['read'], encryption_context: { service_id: 'service' },
    initiated_at: '2026-01-01T00:00:00Z', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
  },
  dependent_agents: [{ id: 'helper', display_name: 'Helper' }], dependent_agent_count: 1,
};
afterEach(() => { apiClient.defaults.adapter = adapter; });

describe('session transport ownership', () => {
  it('reads new server lists and details rather than a principal-independent service cache', async () => {
    let id = 'alice-session';
    apiClient.defaults.adapter = async (config) => ({ config, data: { data: config.url === '/third-party/sessions' ? { sessions: [{ ...summary, id }] } : { ...detail, session: { ...detail.session, id } } }, status: 200, statusText: 'OK', headers: {} });
    expect((await sessionsApi.listSessions())[0].id).toBe('alice-session');
    expect(await sessionsApi.getSessionDetails('service')).toEqual(detail);
    id = 'bob-session';
    expect((await sessionsApi.listSessions())[0].id).toBe('bob-session');
    expect((await sessionsApi.getSessionDetails('service')).session.id).toBe('bob-session');
  });

  it.each([
    ['list', (signal: AbortSignal) => sessionsApi.listSessions({ signal }), { sessions: [] }],
    ['detail', (signal: AbortSignal) => sessionsApi.getSessionDetails('service', { signal }), detail],
  ] as const)('aborts the active session %s request', async (_, read, data) => {
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
