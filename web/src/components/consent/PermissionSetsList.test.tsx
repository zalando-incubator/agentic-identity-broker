/**
 * Regression tests for PermissionSetsList per-service toggle behaviour.
 *
 * Covers the scenario that regressed twice:
 * - First toggle inside a mandatory PS must NOT drop other included services
 * - Effectively-mandatory services (ServiceScope.requirement_type=mandatory)
 *   must remain included even when the user toggles an optional service
 */

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { PermissionSetsList } from './PermissionSetsList';

const MANDATORY_PS_ID = 'ps-mandatory-1';
const SERVICE_A = 'svc-a'; // ServiceScope.requirement_type=mandatory, SR-optional
const SERVICE_B = 'svc-b'; // SR mandatory
const SERVICE_C = 'svc-c'; // fully optional (will be toggled off)
const SERVICE_D = 'svc-d'; // fully optional (must survive toggle of C)

const mandatoryPS = {
  permission_set: {
    id: MANDATORY_PS_ID,
    name: 'Core Access',
    description: 'Core permissions',
    service_scopes: [
      { service_id: SERVICE_A, requirement_type: 'mandatory' as const },
      { service_id: SERVICE_B, requirement_type: 'optional' as const },
      { service_id: SERVICE_C, requirement_type: 'optional' as const },
      { service_id: SERVICE_D, requirement_type: 'optional' as const },
    ],
  },
  requirement_type: 'mandatory' as const,
};

const serviceRequirements = [
  { service_id: SERVICE_A, requirement_type: 'optional' as const }, // PS-mandatory, SR-optional
  { service_id: SERVICE_B, requirement_type: 'mandatory' as const }, // SR-mandatory
  { service_id: SERVICE_C, requirement_type: 'optional' as const }, // fully optional
  { service_id: SERVICE_D, requirement_type: 'optional' as const }, // fully optional
];

const availableServices = [
  { id: SERVICE_A, display_name: 'Service A' },
  { id: SERVICE_B, display_name: 'Service B' },
  { id: SERVICE_C, display_name: 'Service C' },
  { id: SERVICE_D, display_name: 'Service D' },
];

describe('PermissionSetsList — PS-only agent (no service_requirements)', () => {
  it('omits an optional ServiceScope from the grant when the user toggles it off', () => {
    const PS_ID = 'ps-only-1';
    const SVC_LOCK = 'svc-locked'; // ServiceScope.requirement_type=mandatory → always included
    const SVC_OPT = 'svc-opt';    // ServiceScope.requirement_type=optional  → user-removable
    const onSelectionChange = vi.fn();

    render(
      <PermissionSetsList
        permissionSets={[
          {
            permission_set: {
              id: PS_ID,
              name: 'PS Only',
              description: 'no SR',
              service_scopes: [
                { service_id: SVC_LOCK, requirement_type: 'mandatory' as const },
                { service_id: SVC_OPT,  requirement_type: 'optional'  as const },
              ],
            },
            requirement_type: 'mandatory' as const,
          },
        ]}
        availableServices={[
          { id: SVC_LOCK, display_name: 'Locked Service' },
          { id: SVC_OPT,  display_name: 'Optional Service' },
        ]}
        serviceRequirements={[]}
        onSelectionChange={onSelectionChange}
      />,
    );

    // Toggle the optional service off
    const toggle = screen.getByRole('switch', { name: /Optional Service/i });
    fireEvent.click(toggle);

    const lastCall = onSelectionChange.mock.calls[onSelectionChange.mock.calls.length - 1];
    const included: string[] = (lastCall[1] as Record<string, string[]>)[PS_ID] ?? [];

    expect(included).toContain(SVC_LOCK);
    expect(included).not.toContain(SVC_OPT);
  });
});

describe('PermissionSetsList — per-service toggle regression', () => {
  it('preserves all other included services when toggling one optional service in a mandatory PS', () => {
    const onSelectionChange = vi.fn();

    render(
      <PermissionSetsList
        permissionSets={[mandatoryPS]}
        availableServices={availableServices}
        serviceRequirements={serviceRequirements}
        onSelectionChange={onSelectionChange}
      />,
    );

    // Find the toggle for Service C (optional) and turn it off
    const serviceCToggle = screen.getByRole('switch', { name: /Service C/i });
    fireEvent.click(serviceCToggle);

    expect(onSelectionChange).toHaveBeenCalled();
    const lastCall = onSelectionChange.mock.calls[onSelectionChange.mock.calls.length - 1];
    const perPsIncluded: Record<string, string[]> = lastCall[1];
    const includedInPS = perPsIncluded[MANDATORY_PS_ID] ?? [];

    // Service A (PS-mandatory) and Service B (SR-mandatory) must still be included
    expect(includedInPS).toContain(SERVICE_A);
    expect(includedInPS).toContain(SERVICE_B);
    // Service D (untouched optional) must also still be included
    expect(includedInPS).toContain(SERVICE_D);
    // Service C was toggled off
    expect(includedInPS).not.toContain(SERVICE_C);
  });

  it('cannot toggle off a service with ServiceScope.requirement_type=mandatory', () => {
    const onSelectionChange = vi.fn();

    render(
      <PermissionSetsList
        permissionSets={[mandatoryPS]}
        availableServices={availableServices}
        serviceRequirements={serviceRequirements}
        onSelectionChange={onSelectionChange}
      />,
    );

    // Service A is PS-mandatory — it should be rendered as a locked chip, not a toggle
    expect(screen.queryByRole('switch', { name: /Service A/i })).toBeNull();
    // Locked chip should be present
    expect(screen.getByLabelText(/Service A \(required\)/i)).toBeTruthy();
  });
});

describe('PermissionSetsList — requested-service candidates (Feature 055)', () => {
  it('marks covering sets without selecting or locking optional sets', () => {
    const optionalPS = {
      permission_set: {
        id: 'ps-triage',
        name: 'Issue Triage',
        description: 'Triage issues',
        service_scopes: [{ service_id: SERVICE_C, requirement_type: 'optional' as const }],
      },
      requirement_type: 'optional' as const,
    };
    const onSelectionChange = vi.fn();
    render(
      <PermissionSetsList
        permissionSets={[mandatoryPS, optionalPS]}
        availableServices={availableServices}
        serviceRequirements={serviceRequirements}
        requestedCandidates={{ 'ps-triage': ['Service C'] }}
        onSelectionChange={onSelectionChange}
      />,
    );
    const badges = screen.getAllByTestId('requested-candidate-badge');
    expect(badges).toHaveLength(1);
    expect(badges[0]).toHaveTextContent('Covers requested Service C');
    const toggle = screen.getByRole('switch', { name: 'Toggle Issue Triage' });
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(toggle).not.toBeDisabled();
  });
});
