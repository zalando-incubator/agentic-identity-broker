import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { createConsentDraft } from './consentDraft';
import { PermissionGroupList } from './PermissionGroupList';
import type { AgentDetail, ServiceRequirement } from '../../types/consent';

it.each([false, true])('preserves untouched services and effective required locks when removing one service (requirements=%s)', async (withRequirements) => {
  const submit = vi.fn();
  const requirements: AgentDetail['service_requirements'] = withRequirements ? [
    { service_id: 'a', requirement_type: 'optional' }, { service_id: 'b', requirement_type: 'mandatory' },
    { service_id: 'c', requirement_type: 'optional' }, { service_id: 'd', requirement_type: 'optional' },
  ] : [];
  const services: ServiceRequirement[] = ['a', 'b', 'c', 'd'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: `Service ${id}`, requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' }));
  function Editor() {
    const [draft, setDraft] = useState(() => createConsentDraft({ context: 'console', serviceRequirements: requirements, permissionSets: [{ requirement_type: 'mandatory', permission_set: { id: 'core', name: 'Core access', description: 'Read service data.', service_scopes: [
      { service_id: 'a', requirement_type: 'mandatory' }, { service_id: 'b', requirement_type: 'optional' },
      { service_id: 'c', requirement_type: 'optional' }, { service_id: 'd', requirement_type: 'optional' },
    ] } }] }));
    return <><PermissionGroupList draft={draft} services={services} onChange={setDraft} /><button onClick={() => submit(draft.toGrantRequest())}>Submit selection</button></>;
  }
  render(<Editor />);
  await userEvent.click(screen.getByRole('button', { name: 'Core access' }));
  expect(screen.getByRole('checkbox', { name: 'Service a' })).toBeDisabled();
  if (withRequirements) expect(screen.getByRole('checkbox', { name: 'Service b' })).toBeDisabled();
  await userEvent.click(screen.getByRole('checkbox', { name: 'Service c' }));
  expect(screen.getByRole('checkbox', { name: 'Service d' })).toBeChecked();
  await userEvent.click(screen.getByRole('button', { name: 'Submit selection' }));
  expect(submit).toHaveBeenCalledWith({ granted_permission_sets: { core: ['a', 'b', 'd'] } });
});
