import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from './client';
import { apiCache } from './cache';
import { SessionsApiService, type SessionSummary } from './sessions';

vi.mock('./client', () => ({ apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() } }));

const source = '<svg xmlns="http://www.w3.org/2000/svg"><text>Å &amp; 雲</text></svg>';
const base: SessionSummary = {
  id: 'session', service_id: 'atlas', service_display_name: 'Atlas Files',
  token_type: 'Bearer', scope: ['files.read'], initiated_at: '2026-11-18T10:00:00Z',
  requires_reauthentication: false, access_token_expired: false,
  has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};

describe('SessionsApiService current service icons', () => {
  const service = new SessionsApiService();
  beforeEach(() => { vi.clearAllMocks(); apiCache.clear(); });
  afterEach(() => { vi.useRealTimers(); apiCache.clear(); });

  it.each([undefined, null])('replaces cached list and detail metadata with absent source %s after refresh invalidation', async (service_icon_svg) => {
    const original = { ...base, service_icon_svg: source };
    const cleared = service_icon_svg === undefined ? { ...base } : { ...base, service_icon_svg };
    vi.mocked(apiClient.get)
      .mockResolvedValueOnce({ data: { data: { sessions: [original] } } })
      .mockResolvedValueOnce({ data: { data: { session: original, dependent_agents: [] } } })
      .mockResolvedValueOnce({ data: { data: { sessions: [cleared] } } })
      .mockResolvedValueOnce({ data: { data: { session: cleared, dependent_agents: [] } } });
    expect((await service.listSessions())[0].service_icon_svg).toBe(source);
    expect((await service.getSessionDetails('atlas')).session.service_icon_svg).toBe(source);
    await service.listSessions();
    expect(apiClient.get).toHaveBeenCalledTimes(2);
    vi.mocked(apiClient.post).mockResolvedValue({ data: { data: cleared } });
    expect((await service.refreshSession('atlas')).service_icon_svg).toBe(service_icon_svg);
    expect((await service.listSessions())[0].service_icon_svg).toBe(service_icon_svg);
    expect((await service.getSessionDetails('atlas')).session.service_icon_svg).toBe(service_icon_svg);
    expect(apiClient.get).toHaveBeenCalledTimes(4);
  });

  it('retains the two-minute cache and then returns exact current source', async () => {
    vi.useFakeTimers();
    vi.mocked(apiClient.get)
      .mockResolvedValueOnce({ data: { data: { sessions: [base] } } })
      .mockResolvedValueOnce({ data: { data: { sessions: [{ ...base, service_icon_svg: source }] } } });
    await service.listSessions();
    vi.advanceTimersByTime(2 * 60 * 1000);
    expect((await service.listSessions())[0].service_icon_svg).toBeUndefined();
    expect(apiClient.get).toHaveBeenCalledOnce();
    vi.advanceTimersByTime(1);
    expect((await service.listSessions())[0].service_icon_svg).toBe(source);
    expect(apiClient.get).toHaveBeenCalledTimes(2);
  });
});
