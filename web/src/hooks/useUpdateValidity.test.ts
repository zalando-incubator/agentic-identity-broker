import { describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useUpdateValidity } from './useUpdateValidity';
import type { UserGrant } from '../types/consent';

const grant: UserGrant = {
  id: 'grant-1',
  agent_id: 'agent-1',
  principal: 'user@example.com',
  granted_permission_sets: {},
  valid_until: '2030-01-15T00:00:00Z',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('useUpdateValidity', () => {
  it('hydrates the end date when an existing grant loads after the first render', () => {
    const { result, rerender } = renderHook(
      ({ currentGrant }: { currentGrant: UserGrant | null }) =>
        useUpdateValidity(currentGrant),
      { initialProps: { currentGrant: null } },
    );

    rerender({ currentGrant: grant });

    expect(result.current.validityState).toEqual({
      noExpiration: false,
      expiresAt: new Date('2030-01-15T00:00:00Z'),
    });
    expect(result.current.getValidUntil()).toBe('2030-01-15T00:00:00.000Z');
  });

  it('preserves an edited end date when equivalent grant data is refreshed', () => {
    const { result, rerender } = renderHook(
      ({ currentGrant }: { currentGrant: UserGrant }) =>
        useUpdateValidity(currentGrant),
      { initialProps: { currentGrant: grant } },
    );
    const editedEndDate = new Date('2031-02-16T00:00:00Z');

    act(() => {
      result.current.setValidityState({
        noExpiration: false,
        expiresAt: editedEndDate,
      });
    });
    rerender({ currentGrant: { ...grant } });

    expect(result.current.validityState).toEqual({
      noExpiration: false,
      expiresAt: editedEndDate,
    });
    expect(result.current.getValidUntil()).toBe('2031-02-16T00:00:00.000Z');
  });

  it('rehydrates the end date when the saved validity changes', () => {
    const { result, rerender } = renderHook(
      ({ currentGrant }: { currentGrant: UserGrant }) =>
        useUpdateValidity(currentGrant),
      { initialProps: { currentGrant: grant } },
    );

    rerender({
      currentGrant: { ...grant, valid_until: '2032-03-17T00:00:00Z' },
    });

    expect(result.current.getValidUntil()).toBe('2032-03-17T00:00:00.000Z');
  });

  it('explains how to correct a missing selected end date', () => {
    const { result } = renderHook(() => useUpdateValidity());

    act(() => {
      result.current.setValidityState({
        noExpiration: false,
        expiresAt: undefined,
      });
    });

    expect(result.current.validate()).toBe(
      'Enter an end date when Specific end date is selected.',
    );
  });
});
