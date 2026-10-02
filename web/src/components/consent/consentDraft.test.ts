import { describe, expect, it } from 'vitest';
import type { ResolvedPermissionSetEntry, UserGrant } from '../../types/consent';
import { createConsentDraft, resolveValidUntil } from './consentDraft';

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
    const indefinite = createConsentDraft({ permissionSets, existingGrant: { ...existingGrant, valid_until: null }, context: 'decision' });
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

  it('roundtrips selections, custom duration, and the unchanged authorization reference through consent_state', () => {
    const loaded = { permissionSets, serviceRequirements, existingGrant, context: 'decision' as const, sessionToken: 'session' };
    const draft = createConsentDraft(loaded).setPermissionSet('extra', true).setService('extra', 'calendar', false).setDuration('custom', '2031-02-04');
    const restored = createConsentDraft({ ...loaded, consentState: draft.toConsentState() });
    expect(restored.toGrantRequest(now)).toEqual(draft.toGrantRequest(now));
    expect(restored.customDate).toBe('2031-02-04');
    expect(restored.sessionToken).toBe('session');
    expect(restored.reset().toGrantRequest(now).valid_until).toBe(existingGrant.valid_until);
  });

  it('restores canonical callback selections and keeps explicit empty selections', () => {
    const consentState = btoa(JSON.stringify({ selections: { extra: ['mail'] }, duration: 'until-revoked', customDate: '' }));
    const restored = createConsentDraft({ permissionSets, serviceRequirements, context: 'console', consentState });
    expect(restored.toGrantRequest(now).granted_permission_sets).toEqual({ base: ['profile', 'mail'], extra: ['mail'] });
    const optionalOnly = [permissionSets[0]!];
    const draft = createConsentDraft({ permissionSets: optionalOnly, existingGrant: { ...existingGrant, granted_permission_sets: { extra: ['mail'] } }, context: 'console' }).setPermissionSet('extra', false);
    expect(createConsentDraft({ permissionSets: optionalOnly, existingGrant: { ...existingGrant, granted_permission_sets: { extra: ['mail'] } }, context: 'console', consentState: draft.toConsentState() }).selections).toEqual({});
  });

  it('ignores malformed callback state and does not allow it to erase locked prior access', () => {
    const loaded = { permissionSets, serviceRequirements, existingGrant, context: 'decision' as const };
    expect(createConsentDraft({ ...loaded, consentState: 'not-json' }).toGrantRequest(now)).toEqual(createConsentDraft(loaded).toGrantRequest(now));
    const tampered = btoa(JSON.stringify({ selections: { base: [], malicious: ['anything'] }, duration: 'custom', customDate: '2030-08-19' }));
    expect(createConsentDraft({ ...loaded, consentState: tampered }).toGrantRequest(now).granted_permission_sets).toEqual(existingGrant.granted_permission_sets);
    const obsolete = btoa(JSON.stringify({ extra: ['mail'] }));
    expect(createConsentDraft({ ...loaded, consentState: obsolete }).toGrantRequest(now)).toEqual(createConsentDraft(loaded).toGrantRequest(now));
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
