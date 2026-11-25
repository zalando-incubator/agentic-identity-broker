import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from './client';
import { apiCache } from './cache';
import { ConsentApiService } from './consent';
import { SessionsApiService, type SessionSummary } from './sessions';
import type { GetAgentDetailResponse } from '../../types/consent';

vi.mock('./client', () => ({
  apiClient: { get: vi.fn() },
}));

const now = Date.parse('2026-11-04T10:00:00Z');

function session(serviceId: string, requiresReauthentication = false): SessionSummary {
  return {
    id: `${serviceId}-session`,
    service_id: serviceId,
    service_display_name: `${serviceId} Production`,
    token_type: 'Bearer',
    scope: ['read'],
    initiated_at: '2026-11-04T09:00:00Z',
    requires_reauthentication: requiresReauthentication,
    access_token_expired: false,
    has_refresh_token: true,
    dependent_agent_count: 1,
    is_encrypted: true,
  };
}

function agentDetail(reportsConnected: boolean): GetAgentDetailResponse {
  return {
    data: {
      agent: {
        agentId: 'reports-reader',
        display_name: 'Reports Reader',
        description: 'Read reports',
        created_at: '2026-11-04T09:00:00Z',
        updated_at: '2026-11-04T09:00:00Z',
      },
      services: [
        {
          kind: 'requirement',
          serviceId: 'reports',
          serviceName: 'Reports Production',
          requirementType: 'mandatory',
          requiredScopes: [{ name: 'read' }],
          connectionStatus: reportsConnected ? 'connected' : 'not_connected',
        },
        {
          kind: 'requirement',
          serviceId: 'calendar',
          serviceName: 'Calendar Production',
          requirementType: 'optional',
          requiredScopes: [{ name: 'read' }],
          connectionStatus: 'connected',
        },
      ],
    },
  };
}

describe('live connection usability with the real API cache', () => {
  beforeEach(() => {
    apiCache.clear();
    vi.mocked(apiClient.get).mockReset();
    vi.spyOn(Date, 'now').mockReturnValue(now);
  });

  afterEach(() => {
    apiCache.clear();
    vi.restoreAllMocks();
  });

  it('reloads session summaries inside the former two-minute TTL without changing Calendar', async () => {
    const service = new SessionsApiService();
    const calendar = session('calendar');
    vi.mocked(apiClient.get)
      .mockResolvedValueOnce({ data: { data: { sessions: [session('reports'), calendar] } } })
      .mockResolvedValueOnce({ data: { data: { sessions: [session('reports', true), calendar] } } });

    const first = await service.listSessions();
    vi.mocked(Date.now).mockReturnValue(now + 1000);
    const second = await service.listSessions();

    expect(first[0].requires_reauthentication).toBe(false);
    expect(second[0].requires_reauthentication).toBe(true);
    expect(second[0].access_token_expired).toBe(false);
    expect(second[1]).toEqual(first[1]);
    expect(apiClient.get).toHaveBeenCalledTimes(2);
    expect(apiClient.get).toHaveBeenLastCalledWith('/third-party/sessions');
  });

  it('reloads session detail inside the former two-minute TTL', async () => {
    const service = new SessionsApiService();
    const dependentAgents = [{ id: 'reports-reader', display_name: 'Reports Reader' }];
    vi.mocked(apiClient.get)
      .mockResolvedValueOnce({ data: { data: { session: session('reports'), dependent_agents: dependentAgents } } })
      .mockResolvedValueOnce({ data: { data: { session: session('reports', true), dependent_agents: dependentAgents } } });

    const first = await service.getSessionDetails('reports');
    vi.mocked(Date.now).mockReturnValue(now + 1000);
    const second = await service.getSessionDetails('reports');

    expect(first.session.requires_reauthentication).toBe(false);
    expect(second.session.requires_reauthentication).toBe(true);
    expect(second.session.access_token_expired).toBe(false);
    expect(second.dependent_agents).toEqual(first.dependent_agents);
    expect(apiClient.get).toHaveBeenCalledTimes(2);
    expect(apiClient.get).toHaveBeenLastCalledWith('/third-party/reports/session');
  });

  it.each([undefined, { sessionToken: 'sealed/context+token' }])(
    'reloads consent connection state inside the former five-minute TTL with options %j',
    async (options) => {
      const service = new ConsentApiService();
      vi.mocked(apiClient.get)
        .mockResolvedValueOnce({ data: agentDetail(true) })
        .mockResolvedValueOnce({ data: agentDetail(false) });

      const first = await service.getAgentDetail('reports-reader', options);
      vi.mocked(Date.now).mockReturnValue(now + 1000);
      const second = await service.getAgentDetail('reports-reader', options);

      expect(first.services[0]).toMatchObject({ connectionStatus: 'connected' });
      expect(second.services[0]).toMatchObject({ connectionStatus: 'not_connected' });
      expect(second.services[1]).toEqual(first.services[1]);
      expect(second.agent).toEqual(first.agent);
      expect(apiClient.get).toHaveBeenCalledTimes(2);
      expect(apiClient.get).toHaveBeenLastCalledWith(
        options
          ? '/consent/agents/reports-reader?session_token=sealed%2Fcontext%2Btoken'
          : '/consent/agents/reports-reader',
      );
    },
  );

  it('retains metadata and grant cache entries across live usability reads', async () => {
    const service = new ConsentApiService();
    const user = { principal: 'alice@example.com' };
    const delegations = [{ agentId: 'reports-reader' }];
    const grant = { id: 'grant-1' };
    apiCache.set('/me', user);
    apiCache.set('/consent/agents', delegations);
    apiCache.set('/consent/agents/reports-reader/grants', grant);
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: agentDetail(false) });

    await service.getAgentDetail('reports-reader');

    expect(await service.getUserInfo()).toEqual(user);
    expect(await service.getAgentDelegations()).toEqual(delegations);
    expect(await service.getAgentGrants('reports-reader')).toEqual(grant);
    expect(apiClient.get).toHaveBeenCalledTimes(1);
  });
});
