import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearPendingConsentSelections,
  loadConsentSelections,
  loadPendingConsentSelections,
  saveConsentSelections,
  type ConsentSelections,
} from './session';

const storagePrefix = 'agentic-identity-broker:consent-state:';
const recordLifetimeMilliseconds = 15 * 60 * 1000;
const testTime = new Date('2026-09-22T12:00:00Z');
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

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
    const stateID = saveConsentSelections(largeSelections);

    expect(stateID).toMatch(uuidPattern);
    expect(stateID).toHaveLength(36);
    expect(
      JSON.parse(sessionStorage.getItem(`${storagePrefix}${stateID}`) ?? ''),
    ).toEqual({
      expiresAt: Date.now() + recordLifetimeMilliseconds,
      selections: largeSelections,
    });

    expect(
      sessionStorage.getItem('agentic-identity-broker:pending-consent-state'),
    ).toBe(stateID);
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

    expect(saveConsentSelections(largeSelections)).toMatch(uuidPattern);
    expect(
      sessionStorage.getItem(`${storagePrefix}${expiredStateID}`),
    ).toBeNull();
  });

  it('loads a validated selection map from a current-tab record', () => {
    const stateID = 'd0000000-0000-4000-8000-000000000001';
    const selections: ConsentSelections = {
      'permission-set-1': ['service-1', 'service-2'],
    };
    sessionStorage.setItem(
      `${storagePrefix}${stateID}`,
      JSON.stringify({
        expiresAt: Date.now() + recordLifetimeMilliseconds,
        selections,
      }),
    );

    expect(loadConsentSelections(stateID)).toEqual(selections);
  });

  it('loads a valid single-service selection map', () => {
    const stateID = 'd0000000-0000-4000-8000-000000000007';
    const selections: ConsentSelections = {
      'permission-set-1': ['service-1'],
    };
    sessionStorage.setItem(
      `${storagePrefix}${stateID}`,
      JSON.stringify({
        expiresAt: Date.now() + recordLifetimeMilliseconds,
        selections,
      }),
    );

    expect(loadConsentSelections(stateID)).toEqual(selections);
  });

  it('loads and clears the pending current-tab selection reference', () => {
    const stateID = saveConsentSelections(largeSelections);

    expect(loadPendingConsentSelections()).toEqual(largeSelections);
    clearPendingConsentSelections();
    expect(
      sessionStorage.getItem('agentic-identity-broker:pending-consent-state'),
    ).toBeNull();
    expect(loadConsentSelections(stateID ?? '')).toEqual(largeSelections);
  });

  it.each([
    ['an unknown reference', 'd0000000-0000-4000-8000-000000000002', undefined],
    ['a malformed stored record', 'd0000000-0000-4000-8000-000000000003', '{'],
    [
      'an expired record',
      'd0000000-0000-4000-8000-000000000004',
      JSON.stringify({
        expiresAt: testTime.getTime() - 1,
        selections: { 'permission-set-1': ['service-1'] },
      }),
    ],
    [
      'a record with a non-string selection ID',
      'd0000000-0000-4000-8000-000000000005',
      JSON.stringify({
        expiresAt: testTime.getTime() + recordLifetimeMilliseconds,
        selections: { 'permission-set-1': ['service-1', 2] },
      }),
    ],
  ])('returns undefined for %s', (_description, stateID, record) => {
    if (record) {
      sessionStorage.setItem(`${storagePrefix}${stateID}`, record);
    }

    expect(loadConsentSelections(stateID)).toBeUndefined();
  });

  it('returns undefined without throwing when browser storage is unavailable', () => {
    vi.spyOn(window, 'sessionStorage', 'get').mockImplementation(() => {
      throw new Error('storage unavailable');
    });

    expect(() => saveConsentSelections(largeSelections)).not.toThrow();
    expect(saveConsentSelections(largeSelections)).toBeUndefined();
    expect(() =>
      loadConsentSelections('d0000000-0000-4000-8000-000000000006'),
    ).not.toThrow();
    expect(
      loadConsentSelections('d0000000-0000-4000-8000-000000000006'),
    ).toBeUndefined();
  });
});
