import { CanceledError } from 'axios';
import { afterEach, describe, expect, it } from 'vitest';
import { apiClient } from './client';
import { sessionsApi } from './sessions';

const adapter = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = adapter; });

describe('session transport ownership', () => {
  it('reads new server lists and details rather than a principal-independent service cache', async () => {
    let id = 'alice-session';
    apiClient.defaults.adapter = async (config) => ({ config, data: { data: config.url === '/third-party/sessions' ? { sessions: [{ id }] } : { session: { id }, dependent_agents: [] } }, status: 200, statusText: 'OK', headers: {} });
    expect((await sessionsApi.listSessions())[0].id).toBe('alice-session');
    expect((await sessionsApi.getSessionDetails('service')).session.id).toBe('alice-session');
    id = 'bob-session';
    expect((await sessionsApi.listSessions())[0].id).toBe('bob-session');
    expect((await sessionsApi.getSessionDetails('service')).session.id).toBe('bob-session');
  });

  it.each([
    ['list', (signal: AbortSignal) => sessionsApi.listSessions({ signal }), { sessions: [] }],
    ['detail', (signal: AbortSignal) => sessionsApi.getSessionDetails('service', { signal }), { session: { id: 'one' }, dependent_agents: [] }],
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
