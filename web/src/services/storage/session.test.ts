import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { loadConsentDraft, saveConsentDraft } from './session';
import type { ConsentDraftSnapshot } from '@components/consent/consentDraft';

const storagePrefix = 'agentic-identity-broker:consent-state:';
const recordLifetimeMilliseconds = 15 * 60 * 1000;
const testTime = new Date('2026-09-22T12:00:00Z');
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const serviceID = 'd0000000-0000-0000-0000-000000000002';
const returnURL = `${window.location.origin}/agents/agent?session_token=sealed`;
const pathname = '/agents/agent';

const largeDraft: ConsentDraftSnapshot = {
  selections: Object.fromEntries(Array.from({ length: 250 }, (_, index) => [
    `permission-set-${index}`, [`service-${index}-a`, `service-${index}-b`],
  ])),
  duration: 'custom',
  customDate: '2031-02-04',
};

describe('consent draft session storage', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(testTime);
    sessionStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('stores a large canonical draft beneath a fixed-size UUID reference', () => {
    const stateID = saveConsentDraft(largeDraft, serviceID, `${returnURL}&other=value#section`);

    expect(stateID).toMatch(uuidPattern);
    expect(stateID).toHaveLength(36);
    expect(JSON.parse(sessionStorage.getItem(`${storagePrefix}${stateID}`) ?? '')).toEqual({
      expiresAt: Date.now() + recordLifetimeMilliseconds,
      ...largeDraft,
      serviceID,
      returnURL: `${returnURL}&other=value#section`,
    });
  });

  it('expires the reference at exactly fifteen minutes', () => {
    const stateID = saveConsentDraft(largeDraft, serviceID, returnURL)!;
    vi.setSystemTime(testTime.getTime() + recordLifetimeMilliseconds - 1);
    expect(loadConsentDraft(stateID, serviceID, pathname)).toBeDefined();
    vi.setSystemTime(testTime.getTime() + recordLifetimeMilliseconds);
    expect(loadConsentDraft(stateID, serviceID, pathname)).toBeUndefined();
  });
  it('prunes expired consent records before saving a new selection map', () => {
    const expiredStateID = 'd0000000-0000-4000-8000-000000000010';
    sessionStorage.setItem(
      `${storagePrefix}${expiredStateID}`,
      JSON.stringify({
        expiresAt: Date.now() - 1,
        selections: { 'permission-set-1': ['service-1'] },
      }),
    );

    expect(
      saveConsentDraft(largeDraft, serviceID, returnURL),
    ).toMatch(uuidPattern);
    expect(
      sessionStorage.getItem(`${storagePrefix}${expiredStateID}`),
    ).toBeNull();
  });

  it('restores only the matching callback ID, service and page in the originating tab', () => {
    const firstDraft = { selections: { 'permission-set-1': ['service-1'] }, duration: '30-days' as const, customDate: '' };
    const firstID = saveConsentDraft(firstDraft, serviceID, returnURL);
    const secondDraft = { selections: { 'permission-set-2': ['service-2'] }, duration: 'custom' as const, customDate: '2031-02-04' };
    const secondID = saveConsentDraft(secondDraft, serviceID, returnURL);

    expect(loadConsentDraft(firstID ?? '', serviceID, pathname)).toMatchObject(firstDraft);
    expect(loadConsentDraft(secondID ?? '', serviceID, pathname)).toMatchObject(secondDraft);
    expect(loadConsentDraft(firstID ?? '', 'another-service', pathname)).toBeUndefined();
    expect(loadConsentDraft(firstID ?? '', serviceID, '/agents/other')).toBeUndefined();
    sessionStorage.clear();
    expect(loadConsentDraft(firstID ?? '', serviceID, pathname)).toBeUndefined();
  });

  it('rejects a cross-origin return URL even for a valid record', () => {
    const stateID = 'd0000000-0000-4000-8000-000000000007';
    sessionStorage.setItem(
      `${storagePrefix}${stateID}`,
      JSON.stringify({
        expiresAt: Date.now() + recordLifetimeMilliseconds,
        ...largeDraft,
        serviceID,
        returnURL: 'https://other.example.com/agents/agent',
      }),
    );
    expect(loadConsentDraft(stateID, serviceID, pathname)).toBeUndefined();
  });

  it.each([
    ['an unknown reference', 'd0000000-0000-4000-8000-000000000002', undefined],
    ['a malformed reference', '../another-key', undefined],
    ['a malformed stored record', 'd0000000-0000-4000-8000-000000000003', '{'],
    [
      'an expired record',
      'd0000000-0000-4000-8000-000000000004',
      JSON.stringify({
        ...largeDraft,
        expiresAt: testTime.getTime() - 1,
        serviceID,
        returnURL,
      }),
    ],
    [
      'a record with a non-string selection ID',
      'd0000000-0000-4000-8000-000000000005',
      JSON.stringify({
        ...largeDraft,
        expiresAt: testTime.getTime() + recordLifetimeMilliseconds,
        selections: { 'permission-set-1': ['service-1', 2] },
        serviceID,
        returnURL,
      }),
    ],
    [
      'a record with an invalid duration',
      'd0000000-0000-4000-8000-000000000008',
      JSON.stringify({ ...largeDraft, duration: 'invalid', expiresAt: testTime.getTime() + recordLifetimeMilliseconds, serviceID, returnURL }),
    ],
    [
      'a record with a missing custom date',
      'd0000000-0000-4000-8000-000000000009',
      JSON.stringify({ selections: largeDraft.selections, duration: 'custom', expiresAt: testTime.getTime() + recordLifetimeMilliseconds, serviceID, returnURL }),
    ],
  ])('returns undefined for %s', (_description, stateID, record) => {
    if (record) {
      sessionStorage.setItem(`${storagePrefix}${stateID}`, record);
    }

    expect(loadConsentDraft(stateID, serviceID, pathname)).toBeUndefined();
  });

  it('returns undefined when browser storage is unavailable', () => {
    vi.spyOn(window, 'sessionStorage', 'get').mockImplementation(() => {
      throw new Error('storage unavailable');
    });

    expect(
      saveConsentDraft(largeDraft, serviceID, returnURL),
    ).toBeUndefined();
    expect(
      loadConsentDraft('d0000000-0000-4000-8000-000000000006', serviceID, pathname),
    ).toBeUndefined();
  });

  it('does not return a reference when writing storage fails', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage full'); });
    expect(saveConsentDraft(largeDraft, serviceID, returnURL)).toBeUndefined();
  });
});
