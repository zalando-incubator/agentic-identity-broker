import { render, screen, within } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { createConsentDraft } from './consentDraft';
import { PermissionPanel } from './PermissionPanel';

it.each([false, true])('explicitly identifies required console services (single=%s)', (single) => {
  const draft = createConsentDraft({ context: 'console', permissionSets: [{ requirement_type: 'mandatory', permission_set: {
    id: 'read', name: 'Read documents', description: 'Read connected documents.',
    service_scopes: [{ service_id: 'mail', requirement_type: 'mandatory' }, ...(single ? [] : [{ service_id: 'drive', requirement_type: 'optional' as const }])],
  } }] });
  render(<PermissionPanel draft={draft} services={[
    { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', requiredScopes: [], connectionStatus: 'connected' },
    { kind: 'requirement', serviceId: 'drive', serviceName: 'Drive', requirementType: 'optional', requiredScopes: [], connectionStatus: 'connected' },
  ]} mode="console" onChange={vi.fn()} />);
  const group = screen.getByTestId('permission-group');
  const service = within(group).getByText('Mail').closest(single ? 'p' : 'li')!;
  expect(service).toHaveTextContent('Mail — Required');
  expect(within(group).queryByRole('button', { name: 'Mail' })).not.toBeInTheDocument();
  if (!single) expect(within(group).getByRole('button', { name: 'Drive' })).toBeEnabled();
});
