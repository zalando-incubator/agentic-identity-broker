import { CanceledError, type AxiosAdapter } from 'axios';
import { afterEach, describe, expect, it } from 'vitest';
import { ConsentApiService } from './consent';
import { apiClient } from './client';
import type { AgentDelegation, CreateOrUpdateGrantRequest, GetAgentDetailResponse, UserGrant } from '../../types/consent';

const adapter = apiClient.defaults.adapter;
const service = new ConsentApiService();
const grant: UserGrant = { id: 'grant', agent_id: 'agent', principal: 'alice', granted_permission_sets: { read: ['service'] }, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const request: CreateOrUpdateGrantRequest = { granted_permission_sets: grant.granted_permission_sets };
const detail: GetAgentDetailResponse['data'] = {
  agent: { agentId: 'agent', display_name: 'Agent', description: 'Description', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  services: [{ serviceId: 'service', serviceName: 'Service', requirementType: 'mandatory', requiredScopes: [{ name: 'read' }], connectionStatus: 'connected' }],
  permission_sets: [],
  active_session_service_ids: ['service'],
  service_requirements: [{ service_id: 'service', requirement_type: 'mandatory' }],
};
afterEach(() => { apiClient.defaults.adapter = adapter; });

function respond(data: unknown, status = 200): AxiosAdapter {
  return async (config) => ({ config, data, status, statusText: 'OK', headers: {} });
}

describe('consent wire contract', () => {
  it.each([{ grants: [grant] }, { grants: [] }])('reads the documented grants array: $grants', async ({ grants }) => {
    apiClient.defaults.adapter = respond({ data: grants });
    expect(await service.getAgentGrants('agent')).toEqual(grants);
  });

  it('does not reuse a preceding principal response in the transport service', async () => {
    apiClient.defaults.adapter = respond({ data: { principal: 'alice', displayName: 'Alice' } });
    expect((await service.getUserInfo()).principal).toBe('alice');
    apiClient.defaults.adapter = respond({ data: { principal: 'bob', displayName: 'Bob' } });
    expect((await service.getUserInfo()).principal).toBe('bob');
  });

  it('reads updated delegation, agent and grant data without a second cache', async () => {
    apiClient.defaults.adapter = respond({ data: [{ agentId: 'old', displayName: 'Old', activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z' } satisfies AgentDelegation] });
    await service.getAgentDelegations();
    apiClient.defaults.adapter = respond({ data: [{ agentId: 'new', displayName: 'New', activeGrantCount: 1, lastModifiedAt: '2026-01-02T00:00:00Z' } satisfies AgentDelegation] });
    expect(await service.getAgentDelegations()).toEqual([{ agentId: 'new', displayName: 'New', activeGrantCount: 1, lastModifiedAt: '2026-01-02T00:00:00Z' }]);
    apiClient.defaults.adapter = respond({ data: detail });
    await service.getAgentDetail('agent');
    apiClient.defaults.adapter = respond({ data: { ...detail, agent: { ...detail.agent, display_name: 'Renamed' } } });
    expect((await service.getAgentDetail('agent')).agent.displayName).toBe('Renamed');
    apiClient.defaults.adapter = respond({ data: [grant] });
    await service.getAgentGrants('agent');
    apiClient.defaults.adapter = respond({ data: [] });
    expect(await service.getAgentGrants('agent')).toEqual([]);
  });

  it('decodes service requirements with their declared permission ceiling', async () => {
    apiClient.defaults.adapter = respond({ data: detail });
    expect((await service.getAgentDetail('agent')).services).toEqual([{ ...detail.services[0], kind: 'requirement' }]);
  });

  it.each(['https://broker.example/agent.png', null, undefined])('preserves optional nullable logo fields (%s)', async (logoUrl) => {
    const delegation: AgentDelegation = { agentId: 'agent', displayName: 'Agent', logoUrl, activeGrantCount: 1, lastModifiedAt: '2026-01-01T00:00:00Z' };
    apiClient.defaults.adapter = respond({ data: [delegation] });
    expect((await service.getAgentDelegations())[0].logoUrl).toBe(logoUrl);
    apiClient.defaults.adapter = respond({ data: { ...detail, agent: { ...detail.agent, logoUrl } } });
    expect((await service.getAgentDetail('agent')).agent.logoUrl).toBe(logoUrl);
  });

  it('normalizes the supplied client ID without inventing CIMD redirect metadata', async () => {
    apiClient.defaults.adapter = respond({ data: { ...detail, agent: { ...detail.agent, client_id: 'opaque-client' } } });
    const result = await service.getAgentDetail('agent');
    expect(result.agent.clientId).toBe('opaque-client');
    expect(result.cimd_metadata).toBeUndefined();
  });

  it('returns the created grant on 201 without navigating', async () => {
    const location = window.location.href;
    apiClient.defaults.adapter = respond({ data: grant }, 201);
    expect(await service.createOrUpdateGrant('agent', request)).toEqual({ kind: 'created', grant });
    expect(window.location.href).toBe(location);
  });

  it('preserves the authorization session and sibling safe redirect from a 201', async () => {
    const location = window.location.href;
    let url: string | undefined;
    apiClient.defaults.adapter = async (config) => { url = config.url; return { config, data: { data: grant, redirect_url: '/oauth2/authorize?continue=one' }, status: 201, statusText: 'Created', headers: {} }; };
    expect(await service.createOrUpdateGrant('agent', request, { sessionToken: 'session/+&' })).toEqual({ kind: 'redirect', redirectUrl: '/oauth2/authorize?continue=one' });
    expect(url).toBe('/consent/agents/agent/grants?session_token=session%2F%2B%26');
    expect(window.location.href).toBe(location);
  });

  it('rejects an unsafe redirect without changing the browser location', async () => {
    const location = window.location.href;
    apiClient.defaults.adapter = respond({ data: grant, redirect_url: 'https://evil.example/steal' }, 201);
    await expect(service.createOrUpdateGrant('agent', request)).rejects.toThrow('Redirect URL validation failed');
    expect(window.location.href).toBe(location);
  });

  it.each([
    ['identity', (signal: AbortSignal) => service.getUserInfo({ signal }), { principal: 'alice', displayName: 'Alice' }],
    ['delegations', (signal: AbortSignal) => service.getAgentDelegations({ signal }), []],
    ['agent', (signal: AbortSignal) => service.getAgentDetail('agent', { signal, sessionToken: 'session' }), detail],
    ['grant', (signal: AbortSignal) => service.getAgentGrants('agent', { signal }), [grant]],
  ] as const)('cancels an in-flight %s read at the transport', async (_, read, data) => {
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
