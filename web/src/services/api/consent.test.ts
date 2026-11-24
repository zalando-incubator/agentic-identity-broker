import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ConsentApiService } from './consent';
import type { CreateOrUpdateGrantRequest, UserGrant } from '../../types/consent';
import { apiCache } from './cache';

vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

vi.mock('./cache', () => ({
  apiCache: {
    get: vi.fn().mockReturnValue(null),
    set: vi.fn(),
    invalidate: vi.fn(),
    invalidatePattern: vi.fn(),
  },
}));

import { apiClient } from './client';

const mockGrant: UserGrant = {
  id: 'grant-1',
  agent_id: 'agent-1',
  principal: 'user@example.com',
  delegated_oauth2_tokens: [{ thirdparty_oauth2_service_id: 'svc-1', scopes: ['read'] }],
  valid_until: null,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const request: CreateOrUpdateGrantRequest = {
  delegated_oauth2_tokens: [{ thirdparty_oauth2_service_id: 'svc-1', scopes: ['read'] }],
};

describe('ConsentApiService.createOrUpdateGrant', () => {
  let service: ConsentApiService;
  let originalLocation: Location;

  beforeEach(() => {
    service = new ConsentApiService();
    vi.clearAllMocks();

    originalLocation = window.location;
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  it('returns { kind: "created", grant } on 201 without redirect_url', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({
      status: 201,
      data: { data: mockGrant },
    });

    const result = await service.createOrUpdateGrant('agent-1', request);

    expect(result).toEqual({ kind: 'created', grant: mockGrant });
    expect(window.location.href).toBe('');
  });

  it('returns { kind: "redirect", redirectUrl } on 201 with redirect_url and does not navigate', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({
      status: 201,
      data: { data: mockGrant, redirect_url: '/agents/agent-1/done' },
    });

    const result = await service.createOrUpdateGrant('agent-1', request);

    expect(result).toEqual({ kind: 'redirect', redirectUrl: '/agents/agent-1/done' });
    expect(window.location.href).toBe('');
  });

  it('returns { kind: "noContent" } on 204', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({
      status: 204,
      data: {},
    });

    const result = await service.createOrUpdateGrant('agent-1', request);

    expect(result).toEqual({ kind: 'noContent' });
  });

  it('throws on 201 with an unsafe redirect_url', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({
      status: 201,
      data: { data: mockGrant, redirect_url: 'https://evil.example.com/steal' },
    });

    await expect(service.createOrUpdateGrant('agent-1', request)).rejects.toThrow(
      'Redirect URL validation failed',
    );
    expect(window.location.href).toBe('');
  });
});

describe('ConsentApiService.getAgentDetail service icon mapping', () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it.each([undefined, null])('replaces previous source with fresh absence %s and preserves service filtering and cache TTL', async (iconSvg) => {
    const service = new ConsentApiService();
    const source = '<svg xmlns="http://www.w3.org/2000/svg"><text>Å &amp; 雲</text></svg>';
    const entry = {
      serviceId: 'atlas', serviceName: 'Atlas Files', requirementType: 'mandatory',
      requiredScopes: [{ name: 'files.read' }], connectionStatus: 'connected',
    };
    const response = (services: unknown[]) => ({ data: { data: {
      agent: { agentId: 'agent-1', display_name: 'Research', description: 'Research files' },
      services,
      service_requirements: [{ service_id: 'atlas', requirement_type: 'mandatory' }],
    } } });
    vi.mocked(apiClient.get)
      .mockResolvedValueOnce(response([{ ...entry, iconSvg: source }]))
      .mockResolvedValueOnce(response([iconSvg === undefined ? entry : { ...entry, iconSvg }]));
    const first = await service.getAgentDetail('agent-1');
    expect(first.services).toHaveLength(1);
    expect(first.services[0]).toMatchObject({ kind: 'requirement', serviceId: 'atlas', iconSvg: source });
    const next = await service.getAgentDetail('agent-1');
    expect(next.services).toHaveLength(1);
    const nextService = next.services[0];
    if (nextService?.kind !== 'requirement') throw new Error('Expected a service requirement');
    expect(nextService.iconSvg).toBe(iconSvg);
    expect(first.services[0]).toMatchObject({ kind: 'requirement', iconSvg: source });
    expect(apiCache.set).toHaveBeenLastCalledWith('/consent/agents/agent-1', next, 5 * 60 * 1000);
    expect(next.agent.service_requirements).toEqual([{ service_id: 'atlas', requirement_type: 'mandatory' }]);
  });
});
