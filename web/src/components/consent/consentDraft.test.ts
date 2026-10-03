import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ResolvedPermissionSetEntry, UserGrant } from '../../types/consent';
import { createConsentDraft, InvalidGrantDateError, resolveValidUntil } from './consentDraft';

const permissionSets: ResolvedPermissionSetEntry[] = [
  { requirement_type: 'optional', permission_set: { id: 'extra', name: 'Extra access', description: 'Optional work', service_scopes: [{ service_id: 'mail', requirement_type: 'optional' }, { service_id: 'calendar', requirement_type: 'optional' }] } },
  { requirement_type: 'mandatory', permission_set: { id: 'base', name: 'Required access', description: 'Core work', service_scopes: [{ service_id: 'profile', requirement_type: 'mandatory' }, { service_id: 'mail', requirement_type: 'optional' }, { service_id: 'outside', requirement_type: 'optional' }] } },
];
const serviceRequirements = [
  { service_id: 'profile', requirement_type: 'optional' as const },
  { service_id: 'mail', requirement_type: 'mandatory' as const },
  { service_id: 'calendar', requirement_type: 'optional' as const },
];
const existingGrant: UserGrant = {
  id: 'grant', agent_id: 'agent', principal: 'alice',
  granted_permission_sets: { base: ['profile'], retired: ['legacy'] },
  valid_until: '2030-08-19T14:35:00+02:00', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
};
const now = new Date(2026, 8, 27, 12);

describe('consent draft', () => {
  it('orders required groups first and enforces group and effective service locks', () => {
    let draft = createConsentDraft({ permissionSets, serviceRequirements, context: 'console' });
    expect(draft.groups.map(({ id }) => id)).toEqual(['base', 'extra']);
    draft = draft.setPermissionSet('base', false).setService('base', 'profile', false).setService('base', 'mail', false);
    expect(draft.toGrantRequest(now).granted_permission_sets).toEqual({ base: ['profile', 'mail'] });
    expect(draft.groups[0]).toMatchObject({ required: true, readOnly: true, selected: true });
    expect(draft.groups[0]?.services).toEqual([
      { id: 'profile', required: true, selected: true, readOnly: true },
      { id: 'mail', required: true, selected: true, readOnly: true },
    ]);
  });

  it('keeps prior selections read-only without silently adding new services to a granted group', () => {
    let draft = createConsentDraft({ permissionSets, serviceRequirements, existingGrant, context: 'decision', sessionToken: 'session' });
    expect(draft.groups[0]).toMatchObject({ alreadyGranted: true, collapsed: true, selected: true, readOnly: true });
    draft = draft.setPermissionSet('base', false).setService('base', 'mail', true).setPermissionSet('extra', true).setService('extra', 'calendar', false);
    expect(draft.toGrantRequest(now)).toEqual({
      granted_permission_sets: { base: ['profile'], retired: ['legacy'], extra: ['mail'] },
      valid_until: existingGrant.valid_until,
    });
    expect(draft.sessionToken).toBe('session');
    expect(existingGrant.granted_permission_sets).toEqual({ base: ['profile'], retired: ['legacy'] });
  });

  it('keeps an unselected optional group absent and ignores unknown selection identifiers', () => {
    const draft = createConsentDraft({ permissionSets, serviceRequirements, context: 'decision' })
      .setPermissionSet('unknown', true).setService('base', 'outside', true).setService('extra', 'calendar', true);
    expect(draft.toGrantRequest(now).granted_permission_sets).toEqual({ base: ['profile', 'mail'] });
  });

  it('allows console edits to optional existing groups and restores the loaded state on cancel', () => {
    const grant = { ...existingGrant, granted_permission_sets: { base: ['profile', 'mail'], extra: ['mail', 'calendar'] } };
    const loaded = createConsentDraft({ permissionSets, serviceRequirements, existingGrant: grant, context: 'console' });
    expect(loaded.dirty).toBe(false);
    const changed = loaded.setService('extra', 'calendar', false).setDuration('30-days');
    expect(changed.dirty).toBe(true);
    expect(changed.reset().toGrantRequest(now)).toEqual(loaded.toGrantRequest(now));
    expect(changed.reset().dirty).toBe(false);
    expect(loaded.setService('extra', 'calendar', false).setService('extra', 'calendar', true).dirty).toBe(false);
    expect(loaded.setPermissionSet('extra', false).toGrantRequest(now).granted_permission_sets).toEqual({ base: ['profile', 'mail'] });
  });

  it('preserves the exact existing expiry unless the user changes the duration', () => {
    const draft = createConsentDraft({ permissionSets, existingGrant, context: 'decision' });
    expect(draft.duration).toBe('custom');
    expect(draft.customDate).toBe('2030-08-19');
    expect(draft.toGrantRequest(now).valid_until).toBe('2030-08-19T14:35:00+02:00');
    expect(draft.setDuration('30-days').toGrantRequest(now).valid_until).toBe(new Date(now.getTime() + 30 * 86_400_000).toISOString());
    expect(draft.setDuration('until-revoked').toGrantRequest(now)).not.toHaveProperty('valid_until');
    const indefinite = createConsentDraft({ permissionSets, existingGrant: { ...existingGrant, valid_until: undefined }, context: 'decision' });
    expect(indefinite.duration).toBe('until-revoked');
    expect(indefinite.toGrantRequest(now)).not.toHaveProperty('valid_until');
  });
  it('returns to a clean draft and preserves exact validity after a duration round trip', () => {
    const loaded = createConsentDraft({ permissionSets, serviceRequirements, existingGrant, context: 'console' });
    const restored = loaded.setDuration('until-revoked').setDuration('30-days').setDuration('custom');
    expect(restored.duration).toBe(loaded.duration);
    expect(restored.customDate).toBe(loaded.customDate);
    expect(restored.dirty).toBe(false);
    expect(restored.toGrantRequest(now).valid_until).toBe(existingGrant.valid_until);
  });

  it('roundtrips selections, custom duration, and the unchanged authorization reference through a typed snapshot', () => {
    const loaded = { permissionSets, serviceRequirements, existingGrant, context: 'decision' as const, sessionToken: 'session' };
    const draft = createConsentDraft(loaded).setPermissionSet('extra', true).setService('extra', 'calendar', false).setDuration('custom', '2031-02-04');
    const restored = createConsentDraft({ ...loaded, restoredDraft: draft.snapshot() });
    expect(restored.toGrantRequest(now)).toEqual(draft.toGrantRequest(now));
    expect(restored.customDate).toBe('2031-02-04');
    expect(restored.sessionToken).toBe('session');
    expect(restored.reset().toGrantRequest(now).valid_until).toBe(existingGrant.valid_until);
  });

  it('restores partial selections and keeps explicit empty selections', () => {
    const restoredDraft = { selections: { extra: ['mail'] }, duration: 'until-revoked' as const, customDate: '' };
    const restored = createConsentDraft({ permissionSets, serviceRequirements, context: 'console', restoredDraft });
    expect(restored.toGrantRequest(now).granted_permission_sets).toEqual({ base: ['profile', 'mail'], extra: ['mail'] });
    const optionalOnly = [permissionSets[0]!];
    const loaded = { permissionSets: optionalOnly, existingGrant: { ...existingGrant, granted_permission_sets: { extra: ['mail'] } }, context: 'console' as const };
    const draft = createConsentDraft(loaded).setPermissionSet('extra', false);
    expect(createConsentDraft({ ...loaded, restoredDraft: draft.snapshot() }).selections).toEqual({});
  });

  it('normalizes callback selections without allowing an unknown group to erase locked prior access', () => {
    const loaded = { permissionSets, serviceRequirements, existingGrant, context: 'decision' as const };
    const restoredDraft = { selections: { base: [], malicious: ['anything'] }, duration: 'custom' as const, customDate: '2030-08-19' };
    const draft = createConsentDraft({ ...loaded, restoredDraft });
    expect(draft.toGrantRequest(now).granted_permission_sets).toEqual(existingGrant.granted_permission_sets);
    const snapshot = draft.snapshot();
    snapshot.selections.base!.push('malicious');
    expect(draft.toGrantRequest(now).granted_permission_sets).toEqual(existingGrant.granted_permission_sets);
  });
});

describe('grant lookup expiry', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(now);
  });
  afterEach(() => vi.useRealTimers());

  it.each(['decision', 'console'] as const)('starts %s consent without an expired grant’s prior optional access or expiry', (context) => {
    const expiredGrant: UserGrant = {
      ...existingGrant,
      granted_permission_sets: { base: ['profile'], extra: ['calendar'], retired: ['legacy'] },
      valid_until: new Date(now.getTime() - 1).toISOString(),
    };
    const draft = createConsentDraft({ permissionSets, serviceRequirements, existingGrant: expiredGrant, context });

    expect(draft.groups.find(({ id }) => id === 'base')).toMatchObject({ required: true, selected: true, readOnly: true });
    expect(draft.groups.find(({ id }) => id === 'extra')).toMatchObject({ selected: false, alreadyGranted: false, readOnly: false, collapsed: false });
    expect(draft.duration).toBe('until-revoked');
    expect(draft.customDate).toBe('');
    expect(draft.dirty).toBe(false);
    expect(draft.toGrantRequest()).toEqual({ granted_permission_sets: { base: ['profile', 'mail'] } });
    expect(draft.setPermissionSet('extra', true).setService('extra', 'calendar', false).toGrantRequest()).toEqual({
      granted_permission_sets: { base: ['profile', 'mail'], extra: ['mail'] },
    });
    expect(draft.toGrantRequest()).toEqual({ granted_permission_sets: { base: ['profile', 'mail'] } });
  });

  it('treats a grant expiring at the exact lookup instant as inactive', () => {
    const draft = createConsentDraft({
      permissionSets, serviceRequirements, existingGrant: { ...existingGrant, granted_permission_sets: { extra: ['calendar'] }, valid_until: now.toISOString() }, context: 'decision',
    });
    expect(draft.groups.find(({ id }) => id === 'extra')).toMatchObject({ selected: false, alreadyGranted: false, readOnly: false });
    expect(draft.toGrantRequest()).toEqual({ granted_permission_sets: { base: ['profile', 'mail'] } });
  });

  it.each(['decision', 'console'] as const)('preserves %s grant selections and the precise validity while still future', (context) => {
    const validUntil = new Date(now.getTime() + 1).toISOString();
    const grant: UserGrant = { ...existingGrant, granted_permission_sets: { base: ['profile'], extra: ['calendar'] }, valid_until: validUntil };
    const draft = createConsentDraft({ permissionSets, existingGrant: grant, context });
    expect(draft.groups.find(({ id }) => id === 'extra')).toMatchObject({ selected: true, alreadyGranted: true, readOnly: context === 'decision' });
    expect(draft.duration).toBe('custom');
    expect(draft.toGrantRequest()).toEqual({ granted_permission_sets: { base: ['profile'], extra: ['calendar'] }, valid_until: validUntil });
  });

  it.each(['decision', 'console'] as const)('rejects %s submission when an unchanged validity expires during editing', (context) => {
    const validUntil = new Date(now.getTime() + 1).toISOString();
    const draft = createConsentDraft({ permissionSets, existingGrant: { ...existingGrant, valid_until: validUntil }, context }).setPermissionSet('extra', true);
    vi.setSystemTime(new Date(now.getTime() + 1));
    expect(() => draft.toGrantRequest()).toThrow(InvalidGrantDateError);
    expect(draft.setDuration('30-days').toGrantRequest().valid_until).toBe(new Date(now.getTime() + 1 + 30 * 86_400_000).toISOString());
  });

  it('keeps an explicit restored callback selection after an expired lookup without reviving the old grant', () => {
    const restoredDraft = { selections: { extra: ['calendar'], retired: ['legacy'] }, duration: 'custom' as const, customDate: '2031-02-04' };
    const draft = createConsentDraft({
      permissionSets, serviceRequirements, existingGrant: { ...existingGrant, granted_permission_sets: { extra: ['mail'], retired: ['legacy'] }, valid_until: now.toISOString() },
      context: 'decision', restoredDraft,
    });
    expect(draft.toGrantRequest().granted_permission_sets).toEqual({ base: ['profile', 'mail'], extra: ['mail', 'calendar'] });
    expect(draft.reset().toGrantRequest()).toEqual({ granted_permission_sets: { base: ['profile', 'mail'] } });
  });
});

describe('grant duration', () => {
  it('omits indefinite expiry, adds exactly thirty days, and formats custom dates as ISO timestamps', () => {
    expect(resolveValidUntil('until-revoked', undefined, now)).toBeUndefined();
    expect(resolveValidUntil('30-days', undefined, now)).toBe(new Date(now.getTime() + 30 * 86_400_000).toISOString());
    expect(resolveValidUntil('custom', '2026-10-01', now)).toBe(new Date(2026, 9, 1).toISOString());
    const selected = new Date(2026, 9, 1, 14, 5);
    expect(resolveValidUntil('custom', selected, now)).toBe(selected.toISOString());
  });

  it.each([undefined, '', '2026-02-30', 'invalid', '2026-09-26', '2026-09-27'])('rejects a missing, invalid, or not-after-today custom date: %s', (date) => {
    expect(() => resolveValidUntil('custom', date, now)).toThrow();
  });
});
