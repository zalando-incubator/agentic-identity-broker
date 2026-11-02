import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from './client';
import { apiCache } from './cache';
import { SessionsApiService } from './sessions';
import type { SessionDetail, SessionSummary } from './sessions';

vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

const activeSession: SessionSummary = {
  id: 'session-1',
  service_id: 'google',
  service_display_name: 'Google Drive',
  token_type: 'Bearer',
  scope: ['read:email'],
  initiated_at: '2024-01-01T12:00:00Z',
  requires_reauthentication: false,
  access_token_expired: false,
  has_refresh_token: true,
  dependent_agent_count: 0,
  is_encrypted: true,
};

const rejectedSession: SessionSummary = {
  ...activeSession,
  requires_reauthentication: true,
};

const rejection = {
  error: 'refresh_rejected',
  message: 'refresh token rejected by provider; reauthentication required',
};

describe('SessionsApiService.refreshSession', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    apiCache.clear();
  });

  it.each([
    { outcome: 'provider rejection', error: rejection, session: rejectedSession },
    {
      outcome: 'transient failure',
      error: new Error('Network error'),
      session: activeSession,
    },
    {
      outcome: 'success',
      error: null,
      session: { ...activeSession, scope: ['read:email', 'write:files'] },
    },
  ])('refetches cached list and details after $outcome', async ({ error, session }) => {
    const service = new SessionsApiService();
    const activeDetails: SessionDetail = {
      session: activeSession,
      dependent_agents: [],
    };
    const updatedDetails: SessionDetail = { ...activeDetails, session };
    vi.mocked(apiClient.get)
      .mockResolvedValueOnce({ data: { data: { sessions: [activeSession] } } })
      .mockResolvedValueOnce({ data: { data: activeDetails } })
      .mockResolvedValueOnce({ data: { data: { sessions: [session] } } })
      .mockResolvedValueOnce({ data: { data: updatedDetails } });

    expect(await service.listSessions()).toEqual([activeSession]);
    expect(await service.getSessionDetails('google')).toEqual(activeDetails);
    expect(await service.listSessions()).toEqual([activeSession]);
    expect(await service.getSessionDetails('google')).toEqual(activeDetails);
    expect(apiClient.get).toHaveBeenCalledTimes(2);

    if (error) {
      vi.mocked(apiClient.post).mockRejectedValueOnce(error);
      await expect(service.refreshSession('google')).rejects.toBe(error);
    } else {
      vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { data: session } });
      await expect(service.refreshSession('google')).resolves.toEqual(session);
    }

    expect(await service.listSessions()).toEqual([session]);
    expect(await service.getSessionDetails('google')).toEqual(updatedDetails);
    expect(apiClient.get).toHaveBeenCalledTimes(4);
    expect(apiClient.post).toHaveBeenCalledOnce();
  });
});
