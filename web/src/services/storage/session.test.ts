import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  loadConsentState,
  saveConsentSelections,
  type ConsentSelections,
} from './session';

const storagePrefix = 'agentic-identity-broker:consent-state:';
const recordLifetimeMilliseconds = 15 * 60 * 1000;
const testTime = new Date('2026-09-22T12:00:00Z');
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const serviceID = 'd0000000-0000-0000-0000-000000000002';
const returnURL = `${window.location.origin}/agents/agent?session_token=sealed`;
const pathname = '/agents/agent';

const largeSelections: ConsentSelections = Object.fromEntries(
  Array.from({ length: 250 }, (_, index) => [
    `permission-set-${index}`,
    [`service-${index}-a`, `service-${index}-b`],
  ]),
);

describe('consent selection session storage', () => {
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

  it('stores a large selection map beneath a fixed-size UUID reference', () => {
    const stateID = saveConsentSelections(largeSelections, serviceID, returnURL);

    expect(stateID).toMatch(uuidPattern);
    expect(stateID).toHaveLength(36);
    expect(
      JSON.parse(sessionStorage.getItem(`${storagePrefix}${stateID}`) ?? ''),
    ).toEqual({
      expiresAt: Date.now() + recordLifetimeMilliseconds,
      selections: largeSelections,
      serviceID,
      returnURL,
    });
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
      saveConsentSelections(largeSelections, serviceID, returnURL),
    ).toMatch(uuidPattern);
    expect(
      sessionStorage.getItem(`${storagePrefix}${expiredStateID}`),
    ).toBeNull();
  });

  it('restores only the matching callback ID, service and page in the originating tab', () => {
    const firstID = saveConsentSelections(
      { 'permission-set-1': ['service-1'] },
      serviceID,
      returnURL,
    );
    const secondSelections = { 'permission-set-2': ['service-2'] };
    const secondID = saveConsentSelections(secondSelections, serviceID, returnURL);

    expect(loadConsentState(firstID ?? '', serviceID, pathname)?.selections).toEqual({
      'permission-set-1': ['service-1'],
    });
    expect(
      loadConsentState(secondID ?? '', serviceID, pathname)?.selections,
    ).toEqual(secondSelections);
    expect(loadConsentState(firstID ?? '', 'another-service', pathname)).toBeUndefined();
    expect(loadConsentState(firstID ?? '', serviceID, '/agents/other')).toBeUndefined();
    sessionStorage.clear();
    expect(loadConsentState(firstID ?? '', serviceID, pathname)).toBeUndefined();
  });

  it('rejects a cross-origin return URL even for a valid record', () => {
    const stateID = 'd0000000-0000-4000-8000-000000000007';
    sessionStorage.setItem(
      `${storagePrefix}${stateID}`,
      JSON.stringify({
        expiresAt: Date.now() + recordLifetimeMilliseconds,
        selections: largeSelections,
        serviceID,
        returnURL: 'https://other.example.com/agents/agent',
      }),
    );
    expect(loadConsentState(stateID, serviceID, pathname)).toBeUndefined();
  });

  it.each([
    ['an unknown reference', 'd0000000-0000-4000-8000-000000000002', undefined],
    ['a malformed reference', '../another-key', undefined],
    ['a malformed stored record', 'd0000000-0000-4000-8000-000000000003', '{'],
    [
      'an expired record',
      'd0000000-0000-4000-8000-000000000004',
      JSON.stringify({
        expiresAt: testTime.getTime() - 1,
        selections: { 'permission-set-1': ['service-1'] },
        serviceID,
        returnURL,
      }),
    ],
    [
      'a record with a non-string selection ID',
      'd0000000-0000-4000-8000-000000000005',
      JSON.stringify({
        expiresAt: testTime.getTime() + recordLifetimeMilliseconds,
        selections: { 'permission-set-1': ['service-1', 2] },
        serviceID,
        returnURL,
      }),
    ],
  ])('returns undefined for %s', (_description, stateID, record) => {
    if (record) {
      sessionStorage.setItem(`${storagePrefix}${stateID}`, record);
    }

    expect(loadConsentState(stateID, serviceID, pathname)).toBeUndefined();
  });

  it('returns undefined when browser storage is unavailable', () => {
    vi.spyOn(window, 'sessionStorage', 'get').mockImplementation(() => {
      throw new Error('storage unavailable');
    });

    expect(
      saveConsentSelections(largeSelections, serviceID, returnURL),
    ).toBeUndefined();
    expect(
      loadConsentState('d0000000-0000-4000-8000-000000000006', serviceID, pathname),
    ).toBeUndefined();
  });
});
