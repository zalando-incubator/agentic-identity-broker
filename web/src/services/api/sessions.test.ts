import { describe, it, expect, vi, beforeEach } from 'vitest';
import { SessionsApiService } from './sessions';

vi.mock('./client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

vi.mock('./cache', () => ({
  apiCache: { get: vi.fn().mockReturnValue(null), set: vi.fn(), invalidatePattern: vi.fn() },
}));

import { apiClient } from './client';
import { apiCache } from './cache';

describe('SessionsApiService.getServiceSessionDetail', () => {
  beforeEach(() => vi.clearAllMocks());

  it('reads the service detail by identifier without caching it', async () => {
    const data = {
      service: { id: 'svc', canonical_id: 'github', display_name: 'GitHub Production', scope_url: 'https://broker.example.com/sessions/github' },
      session: null,
      dependent_agents: [],
    };
    vi.mocked(apiClient.get).mockResolvedValue({ data: { data } });

    await expect(new SessionsApiService().getServiceSessionDetail('github')).resolves.toEqual(data);
    expect(apiClient.get).toHaveBeenCalledWith('/third-party/sessions/github');
    expect(apiCache.set).not.toHaveBeenCalled();
  });

  it('encodes the identifier as one path segment', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: { data: {} } });
    await new SessionsApiService().getServiceSessionDetail('a/b');
    expect(apiClient.get).toHaveBeenCalledWith('/third-party/sessions/a%2Fb');
  });
});
