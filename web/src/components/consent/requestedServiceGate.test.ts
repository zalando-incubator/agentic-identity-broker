import { describe, it, expect } from 'vitest';
import { evaluateRequestedServices } from './requestedServiceGate';
import type { RequestedServiceCoverage } from '../../types/consent';

const github = 'svc-github';
const names = { [github]: 'GitHub Production' };

const coverage = (overrides: Partial<RequestedServiceCoverage> = {}): RequestedServiceCoverage => ({
  service_id: github,
  required_scopes: ['repo', 'user:email'],
  usability: 'usable',
  covering_permission_sets: [
    { permission_set_id: 'read', supplied_required_scopes: ['repo', 'user:email'], usability: 'usable' },
    { permission_set_id: 'triage', supplied_required_scopes: ['repo'], usability: 'usable' },
  ],
  ...overrides,
});

describe('evaluateRequestedServices', () => {
  it('requires a selected candidate that includes the service', () => {
    const [result] = evaluateRequestedServices([coverage()], { identity: ['svc-identity'] }, names);
    expect(result.satisfied).toBe(false);
    expect(result.reason).toContain('GitHub Production');
    const [excluded] = evaluateRequestedServices([coverage()], { read: [] }, names);
    expect(excluded.satisfied).toBe(false);
  });

  it('explains missing required scopes without implying one candidate suffices', () => {
    const [result] = evaluateRequestedServices([coverage()], { triage: [github] }, names);
    expect(result.satisfied).toBe(false);
    expect(result.reason).toContain('user:email');
  });

  it('accepts complementary selections that cover all required scopes', () => {
    const complementary = coverage({
      covering_permission_sets: [
        { permission_set_id: 'triage', supplied_required_scopes: ['repo'], usability: 'usable' },
        { permission_set_id: 'email', supplied_required_scopes: ['user:email'], usability: 'usable' },
      ],
    });
    expect(evaluateRequestedServices([complementary], { triage: [github], email: [github] }, names)[0].satisfied).toBe(true);
  });

  it('requires the baseline and every selected candidate contribution to be usable', () => {
    expect(evaluateRequestedServices([coverage({ usability: 'missing' })], { read: [github] }, names)[0].reason).toContain('Log in to GitHub Production');
    expect(evaluateRequestedServices([coverage({ usability: 'unavailable' })], { read: [github] }, names)[0].reason).toContain('could not be verified');

    const allScopes = coverage({
      required_scopes: [],
      covering_permission_sets: [
        { permission_set_id: 'read', supplied_required_scopes: [], usability: 'unusable' },
        { permission_set_id: 'triage', supplied_required_scopes: [], usability: 'usable' },
      ],
    });
    expect(evaluateRequestedServices([allScopes], { read: [github] }, names)[0].satisfied).toBe(false);
    expect(evaluateRequestedServices([allScopes], { triage: [github] }, names)[0].satisfied).toBe(true);
    expect(evaluateRequestedServices([allScopes], { triage: [github], read: [github] }, names)[0].satisfied).toBe(false);
  });

  it('is satisfied for a selected, covering and usable candidate', () => {
    const [result] = evaluateRequestedServices([coverage()], { read: [github] }, names);
    expect(result).toEqual({ serviceId: github, serviceName: 'GitHub Production', satisfied: true, reason: '' });
  });
});
