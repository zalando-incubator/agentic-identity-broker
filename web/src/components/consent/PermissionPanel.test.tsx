import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { createConsentDraft } from './consentDraft';
import { PermissionPanel } from './PermissionPanel';
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
    return <><PermissionPanel draft={draft} services={services} onChange={setDraft} mode="console" /><button onClick={() => submit(draft.toGrantRequest())}>Submit selection</button></>;
  }
  render(<Editor />);
  const coreRow = screen.getByTestId('permission-group');
  expect(coreRow).toHaveAttribute('data-selected', 'true');
  expect(coreRow).toHaveAttribute('data-read-only', 'true');
  expect(within(coreRow).queryByRole('checkbox', { name: 'Core access' })).not.toBeInTheDocument();
  expect(within(coreRow).getByText('Required')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Choose services for Core access' })).not.toBeInTheDocument();
  expect(screen.queryByRole('checkbox', { name: 'Service a' })).not.toBeInTheDocument();
  if (withRequirements) expect(screen.queryByRole('button', { name: 'Service b' })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Service c' }));
  expect(screen.getByRole('button', { name: 'Service c' })).toHaveAttribute('aria-pressed', 'false');
  expect(screen.getByRole('button', { name: 'Service d' })).toHaveAttribute('aria-pressed', 'true');
  await userEvent.click(screen.getByRole('button', { name: 'Submit selection' }));
  expect(submit).toHaveBeenCalledWith({ granted_permission_sets: { core: ['a', 'b', 'd'] } });
});

it('keeps already granted decision services locked while showing their names', async () => {
  const services: ServiceRequirement[] = ['a', 'b'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: `Service ${id}`, requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' }));
  const permissionSets: AgentDetail['permission_sets'] = [{ requirement_type: 'optional', permission_set: { id: 'prior', name: 'Prior access', description: 'Existing connection access.', service_scopes: services.map((service) => ({ service_id: service.serviceId, requirement_type: 'optional' })) } }];
  const grant = { id: 'grant', agent_id: 'agent', principal: 'alice', granted_permission_sets: { prior: ['a'] }, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
  const draft = createConsentDraft({ context: 'decision', permissionSets, existingGrant: grant });
  render(<PermissionPanel draft={draft} services={services} onChange={vi.fn()} />);
  expect(screen.getByRole('checkbox', { name: 'Prior access' })).toBeDisabled();
  expect(screen.getByTestId('permission-group-granted')).toHaveTextContent('Granted');
  expect(screen.queryByRole('button', { name: 'Choose services for Prior access' })).not.toBeInTheDocument();
  expect(screen.getByRole('img', { name: 'Service a' })).toBeVisible();
  expect(screen.getByRole('img', { name: 'Service b' })).toBeVisible();
});

it('keeps a selected missing service visible and connectable in its permission row', async () => {
  const connect = vi.fn();
  const services: ServiceRequirement[] = [{ kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', requiredScopes: [], connectionStatus: 'not_connected' }];
  const draft = createConsentDraft({ context: 'decision', permissionSets: [{ requirement_type: 'mandatory', permission_set: { id: 'read', name: 'Read mail', description: 'Reads mail.', service_scopes: [{ service_id: 'mail', requirement_type: 'mandatory' }] } }] });
  render(<PermissionPanel draft={draft} services={services} missingServiceIds={['mail']} onConnect={connect} onChange={vi.fn()} />);
  const row = screen.getByTestId('permission-group');
  expect(within(row).getByRole('img', { name: 'Mail' })).toBeVisible();
  await userEvent.click(within(row).getByRole('button', { name: 'Connect Mail' }));
  expect(connect).toHaveBeenCalledExactlyOnceWith('mail');
});

it.each(['scope', 'agent'] as const)('keeps a %s-required single service selected without a redundant console checkbox', async (source) => {
  const services: ServiceRequirement[] = [{ kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: source === 'agent' ? 'mandatory' : 'optional', requiredScopes: [], connectionStatus: 'connected' }];
  const permissionSets: AgentDetail['permission_sets'] = [{ requirement_type: 'optional', permission_set: { id: 'optional', name: 'Mail access', description: 'Read mail.', service_scopes: [{ service_id: 'mail', requirement_type: source === 'scope' ? 'mandatory' : 'optional' }] } }];
  function Editor() {
    const [draft, setDraft] = useState(() => createConsentDraft({ context: 'console', permissionSets, serviceRequirements: [{ service_id: 'mail', requirement_type: source === 'agent' ? 'mandatory' : 'optional' }] }));
    return <><PermissionPanel draft={draft} services={services} mode="console" onChange={setDraft} /><output data-testid="selected-services">{JSON.stringify(draft.selections)}</output></>;
  }
  render(<Editor />);
  const group = screen.getByRole('checkbox', { name: 'Mail access' });
  expect(screen.getByText('Mail')).toBeVisible();
  expect(screen.getAllByRole('checkbox')).toEqual([group]);
  expect(screen.queryByRole('button', { name: 'Choose services for Mail access' })).not.toBeInTheDocument();
  expect(screen.getByTestId('selected-services')).toHaveTextContent('{}');
  await userEvent.click(screen.getByTestId('permission-group-name'));
  expect(group).toBeChecked();
  expect(screen.getByTestId('permission-group')).toHaveAttribute('data-selected', 'true');
  expect(screen.getByTestId('selected-services')).toHaveTextContent('{"optional":["mail"]}');
});

it('offers a separate Connect action for each missing service in a multi-service group', async () => {
  const connect = vi.fn();
  const services: ServiceRequirement[] = ['mail', 'drive'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: id, requirementType: 'optional', requiredScopes: [], connectionStatus: 'not_connected' }));
  const draft = createConsentDraft({ context: 'decision', permissionSets: [{ requirement_type: 'mandatory', permission_set: { id: 'read', name: 'Read documents', description: 'Read and organize content.', service_scopes: services.map((service) => ({ service_id: service.serviceId, requirement_type: 'optional' })) } }] });
  render(<PermissionPanel draft={draft} services={services} missingServiceIds={['mail', 'drive']} onConnect={connect} onChange={vi.fn()} />);
  await userEvent.click(screen.getByRole('button', { name: 'Choose services for Read documents' }));
  const choices = await screen.findByTestId('permission-services-popover');
  expect(within(choices).getByRole('button', { name: 'Connect mail' })).toBeVisible();
  await userEvent.click(within(choices).getByRole('button', { name: 'Connect drive' }));
  expect(connect).toHaveBeenCalledExactlyOnceWith('drive');
});

it('opens service choices on the first action and returns focus to the row trigger on Escape', async () => {
  const user = userEvent.setup();
  const services: ServiceRequirement[] = ['mail', 'drive'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: id, requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' }));
  const draft = createConsentDraft({ context: 'decision', permissionSets: [{ requirement_type: 'optional', permission_set: { id: 'documents', name: 'Documents', description: 'Access documents.', service_scopes: services.map((service) => ({ service_id: service.serviceId, requirement_type: 'optional' })) } }] });
  render(<PermissionPanel draft={draft} services={services} onChange={vi.fn()} />);
  const trigger = screen.getByRole('button', { name: 'Choose services for Documents' });
  expect(screen.queryByTestId('permission-services-popover')).not.toBeInTheDocument();
  await user.click(trigger);
  const choices = await screen.findByTestId('permission-services-popover');
  expect(within(choices).getByRole('checkbox', { name: 'mail' })).toBeVisible();
  expect(within(choices).getByRole('checkbox', { name: 'drive' })).toBeVisible();
  await user.keyboard('{Escape}');
  await waitFor(() => expect(screen.queryByTestId('permission-services-popover')).not.toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
});

it('keeps console service choices visible and independently keyboard-operable without duplicate checkboxes', async () => {
  const user = userEvent.setup();
  const services: ServiceRequirement[] = ['mail', 'drive'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: id, requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' }));
  function Editor() {
    const [draft, setDraft] = useState(() => createConsentDraft({ context: 'console', permissionSets: [{ requirement_type: 'optional', permission_set: { id: 'documents', name: 'Documents', description: 'Access documents.', service_scopes: services.map((service) => ({ service_id: service.serviceId, requirement_type: 'optional' })) } }] }));
    return <PermissionPanel draft={draft} services={services} mode="console" onChange={setDraft} />;
  }
  render(<Editor />);
  const checkbox = screen.getByRole('checkbox', { name: 'Documents' });
  const mail = screen.getByRole('button', { name: 'mail' });
  expect(mail).toBeVisible();
  expect(mail).toBeDisabled();
  expect(screen.getAllByRole('checkbox')).toEqual([checkbox]);
  await user.click(screen.getByTestId('permission-group-name'));
  expect(checkbox).toBeChecked();
  expect(mail).toHaveAttribute('aria-pressed', 'true');
  mail.focus();
  await user.keyboard('{Enter}');
  expect(mail).toHaveAttribute('aria-pressed', 'false');
  expect(screen.getByRole('button', { name: 'drive' })).toHaveAttribute('aria-pressed', 'true');
  expect(checkbox).toBeChecked();
});

it.each(['decision', 'console'] as const)('toggles a permission through its name and description without opening %s service choices', async (mode) => {
  const user = userEvent.setup();
  const services: ServiceRequirement[] = ['mail', 'drive'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: id, requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' }));
  function Editor() {
    const [draft, setDraft] = useState(() => createConsentDraft({ context: mode, permissionSets: [
      { requirement_type: 'mandatory', permission_set: { id: 'base', name: 'Base access', description: 'Always selected.', service_scopes: [{ service_id: 'mail', requirement_type: 'mandatory' }] } },
      { requirement_type: 'optional', permission_set: { id: 'documents', name: 'Documents', description: 'Access documents.', service_scopes: services.map((service) => ({ service_id: service.serviceId, requirement_type: 'optional' })) } },
    ] }));
    return <PermissionPanel draft={draft} services={services} mode={mode} onChange={setDraft} />;
  }
  render(<Editor />);
  const checkbox = screen.getByRole('checkbox', { name: 'Documents' });
  await user.click(within(checkbox.closest('li')!).getByTestId('permission-group-name'));
  expect(checkbox).toBeChecked();
  if (mode === 'decision') expect(screen.getByRole('button', { name: 'Choose services for Documents' })).toHaveAttribute('aria-expanded', 'false');
  await user.click(screen.getByText('Access documents.'));
  expect(checkbox).not.toBeChecked();
  if (mode === 'decision') expect(screen.getByRole('button', { name: 'Choose services for Documents' })).toHaveAttribute('aria-expanded', 'false');
});

it('expands a clipped description from the keyboard without changing the permission', async () => {
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(80);
  const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(32);
  try {
    const user = userEvent.setup();
    const change = vi.fn();
    const description = 'Read all documents, including private notes and shared files, and send their contents to the agent for analysis.';
    const draft = createConsentDraft({ context: 'decision', permissionSets: [{ requirement_type: 'optional', permission_set: { id: 'documents', name: 'Documents', description, service_scopes: [] } }] });
    render(<PermissionPanel draft={draft} services={[]} onChange={change} />);
    const more = screen.getByRole('button', { name: 'More' });
    more.focus();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('button', { name: 'Less' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText(description)).toBeVisible();
    expect(change).not.toHaveBeenCalled();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('button', { name: 'More' })).toHaveAttribute('aria-expanded', 'false');
  } finally {
    scroll.mockRestore();
    height.mockRestore();
  }
});

it.each(['decision', 'console'] as const)('explains and prevents empty %s selections without treating optional access as required', async (mode) => {
  const user = userEvent.setup();
  const services: ServiceRequirement[] = ['mail', 'drive'].map((id) => ({ kind: 'requirement', serviceId: id, serviceName: id, requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' }));
  function Editor() {
    const [draft, setDraft] = useState(() => createConsentDraft({ context: mode, permissionSets: [{ requirement_type: 'optional', permission_set: { id: 'documents', name: 'Documents', description: 'Access documents.', service_scopes: services.map((service) => ({ service_id: service.serviceId, requirement_type: 'optional' })) } }] }));
    return <><PermissionPanel draft={draft} services={services} mode={mode} onChange={setDraft} /><output aria-label="Grant selection">{JSON.stringify(draft.toGrantRequest().granted_permission_sets)}</output></>;
  }
  render(<Editor />);
  const group = screen.getByRole('checkbox', { name: 'Documents' });
  await user.click(group);
  expect(group).toBeChecked();
  expect(group).toBeDisabled();
  expect(group).toHaveAccessibleDescription(/at least one permission group/);
  expect(screen.queryByText('Required')).not.toBeInTheDocument();
  if (mode === 'decision') await user.click(screen.getByRole('button', { name: 'Choose services for Documents' }));
  const mail = screen.getByRole(mode === 'console' ? 'button' : 'checkbox', { name: 'mail' });
  const drive = screen.getByRole(mode === 'console' ? 'button' : 'checkbox', { name: 'drive' });
  await user.click(mail);
  expect(drive).toBeDisabled();
  expect(drive).toHaveAccessibleDescription(/at least one service/);
  await user.click(drive);
  expect(screen.getByLabelText('Grant selection')).toHaveTextContent('{"documents":["drive"]}');
  if (mode === 'console') expect(screen.getByText(/Revoke access/)).toBeVisible();
  await user.click(mail);
  expect(drive).toBeEnabled();
  await user.click(drive);
  expect(screen.getByLabelText('Grant selection')).toHaveTextContent('{"documents":["mail"]}');
});
