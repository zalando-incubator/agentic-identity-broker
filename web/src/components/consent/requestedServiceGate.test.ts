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
    expect(result.selection).toBe('none');
    expect(result.reason).toBe('To save: turn on a permission set that allows GitHub Production.');
    const [excluded] = evaluateRequestedServices([coverage()], { read: [] }, names);
    expect(excluded.satisfied).toBe(false);
    expect(excluded.selection).toBe('none');
  });

  it('names both unmet conditions in one sentence', () => {
    const [result] = evaluateRequestedServices([coverage({ usability: 'missing' })], {}, names);
    expect(result.login).toBe('missing');
    expect(result.reason).toBe(
      'To save: log in to GitHub Production under Connect Your Accounts and turn on a permission set that allows it.',
    );
  });

  it('names a service without a display name by its identifier', () => {
    const [result] = evaluateRequestedServices([coverage()], {}, {});
    expect(result.serviceName).toBe(github);
    expect(result.reason).toBe(`To save: turn on a permission set that allows ${github}.`);
  });

  it('points to an administrator when no offered permission set covers the service', () => {
    const [result] = evaluateRequestedServices([coverage({ usability: 'missing', covering_permission_sets: [] })], {}, names);
    expect(result.satisfied).toBe(false);
    expect(result.reason).toBe(
      'To save: ask an administrator to add a permission set that allows GitHub Production.',
    );
  });

  it('explains missing required scopes without implying one candidate suffices', () => {
    const [result] = evaluateRequestedServices([coverage()], { triage: [github] }, names);
    expect(result.satisfied).toBe(false);
    expect(result.selection).toBe('incomplete');
    expect(result.missingScopes).toEqual(['user:email']);
    expect(result.reason).toBe(
      'To save: turn on permission sets that together give GitHub Production user:email.',
    );
  });

  it('accepts complementary selections that cover all required scopes', () => {
    const complementary = coverage({
      covering_permission_sets: [
        { permission_set_id: 'triage', supplied_required_scopes: ['repo'], usability: 'usable' },
        { permission_set_id: 'email', supplied_required_scopes: ['user:email'], usability: 'usable' },
      ],
    });
    const [result] = evaluateRequestedServices([complementary], { triage: [github], email: [github] }, names);
    expect(result.satisfied).toBe(true);
    expect(result.selection).toBe('on');
  });

  it('requires the baseline and every selected candidate contribution to be usable', () => {
    const [missing] = evaluateRequestedServices([coverage({ usability: 'missing' })], { read: [github] }, names);
    expect(missing.login).toBe('missing');
    expect(missing.reason).toBe('To save: log in to GitHub Production under Connect Your Accounts.');
    const [unavailable] = evaluateRequestedServices([coverage({ usability: 'unavailable' })], { read: [github] }, names);
    expect(unavailable.login).toBe('unavailable');
    expect(unavailable.reason).toBe('To save: retry the GitHub Production check.');

    const allScopes = coverage({
      required_scopes: [],
      covering_permission_sets: [
        { permission_set_id: 'read', supplied_required_scopes: [], usability: 'unusable' },
        { permission_set_id: 'triage', supplied_required_scopes: [], usability: 'usable' },
      ],
    });
    const [unusable] = evaluateRequestedServices([allScopes], { read: [github] }, names);
    expect(unusable.satisfied).toBe(false);
    expect(unusable.login).toBe('unusable');
    expect(unusable.reason).toBe('To save: log in to GitHub Production under Connect Your Accounts.');
    expect(evaluateRequestedServices([allScopes], { triage: [github] }, names)[0].satisfied).toBe(true);
    expect(evaluateRequestedServices([allScopes], { triage: [github], read: [github] }, names)[0].satisfied).toBe(false);
  });

  it('reports the baseline login state while nothing is selected', () => {
    const [result] = evaluateRequestedServices(
      [coverage({ usability: 'usable', covering_permission_sets: [{ permission_set_id: 'read', supplied_required_scopes: [], usability: 'unusable' }] })],
      {},
      names,
    );
    expect(result.login).toBe('usable');
  });

  it('is satisfied for a selected, covering and usable candidate', () => {
    const [result] = evaluateRequestedServices([coverage()], { read: [github] }, names);
    expect(result).toEqual({
      serviceId: github,
      serviceName: 'GitHub Production',
      satisfied: true,
      login: 'usable',
      selection: 'on',
      missingScopes: [],
      reason: '',
    });
  });
});
