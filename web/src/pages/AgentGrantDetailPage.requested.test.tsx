/**
 * Requested-service consent from specs/055-incremental-service-authorization (FR-009–FR-011).
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { AgentGrantDetailPage } from './AgentGrantDetailPage';
import { ToastProvider } from '../components/ui/Toast';
import * as useAgentGrantsModule from '../hooks/useAgentGrants';
import * as useConsentModule from '../hooks/useConsent';
import type { AgentDetail, RequestedServiceCoverage, ThirdpartyService, UserGrant } from '../types/consent';

vi.mock('../hooks/useAgentGrants');
vi.mock('../hooks/useConsent');
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return { ...actual, useParams: () => ({ agentId: 'agent-1' }) };
});

const github = 'svc-github';
const identity = 'svc-identity';

const permissionSet = (id: string, name: string, serviceID: string, requirement: 'mandatory' | 'optional') => ({
  permission_set: { id, name, description: name, service_scopes: [{ service_id: serviceID, requirement_type: 'optional' as const }] },
  requirement_type: requirement,
});

const agent = (coverage: RequestedServiceCoverage): AgentDetail => ({
  agentId: 'agent-1',
  displayName: 'Data Analysis Assistant',
  description: 'Analyzes repositories',
  permission_sets: [
    permissionSet('identity', 'Identity Access', identity, 'mandatory'),
    permissionSet('read', 'GitHub Read Access', github, 'optional'),
    permissionSet('triage', 'GitHub Issue Triage', github, 'optional'),
  ],
  active_session_service_ids: coverage.usability === 'missing' ? [identity] : [identity, github],
  service_requirements: [
    { service_id: identity, requirement_type: 'mandatory' },
    { service_id: github, requirement_type: 'optional' },
  ],
  requested_service_ids: [github],
  requested_service_coverage: [coverage],
});

const services: ThirdpartyService[] = [
  { kind: 'requirement', serviceId: identity, serviceName: 'Corporate Identity', requirementType: 'mandatory', requiredScopes: [], connectionStatus: 'connected' },
  { kind: 'requirement', serviceId: github, serviceName: 'GitHub Production', requirementType: 'optional', requiredScopes: [], connectionStatus: 'not_connected' },
] as ThirdpartyService[];

const grant: UserGrant = {
  id: 'grant-1', agent_id: 'agent-1', principal: 'alice@example.com',
  granted_permission_sets: { identity: [identity] }, valid_until: null,
  created_at: '2026-10-27T00:00:00Z', updated_at: '2026-10-27T00:00:00Z',
};

const coverage = (usability: RequestedServiceCoverage['usability'], readUsability = usability): RequestedServiceCoverage => ({
  service_id: github,
  required_scopes: ['repo', 'user:email'],
  usability,
  covering_permission_sets: [
    { permission_set_id: 'read', supplied_required_scopes: ['repo', 'user:email'], usability: readUsability },
    { permission_set_id: 'triage', supplied_required_scopes: ['repo'], usability },
  ],
});

const renderWith = (requested: RequestedServiceCoverage) => {
  vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
    agent: agent(requested), services, cimdMeta: null, grants: grant, loading: false, error: null, refetch: vi.fn(),
  } as never);
  render(
    <ToastProvider>
      <MemoryRouter initialEntries={['/agents/agent-1?session_token=sealed']}>
        <Routes>
          <Route path="*" element={<AgentGrantDetailPage />} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  );
};

const saveReason = () => {
  const save = screen.getByRole('button', { name: 'Save' });
  const reasonID = save.getAttribute('aria-describedby');
  return reasonID ? document.getElementById(reasonID)?.textContent ?? '' : '';
};

describe('AgentGrantDetailPage — requested services', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(useConsentModule, 'useConsent').mockReturnValue({ delegations: [], userInfo: null, loading: false, error: null, refetch: vi.fn() } as never);
  });

  const group = () =>
    screen.getByRole('group', { name: 'Data Analysis Assistant is asking for GitHub Production' });

  it('groups covering sets under the request, keeps them unselected and names every unmet condition', async () => {
    const user = userEvent.setup();
    renderWith(coverage('missing'));
    expect(screen.queryByText(/was requested/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Covers requested/)).not.toBeInTheDocument();
    expect(within(group()).getByRole('switch', { name: 'Toggle GitHub Read Access' })).toHaveAttribute('aria-checked', 'false');
    expect(within(group()).getByRole('switch', { name: 'Toggle GitHub Issue Triage' })).toBeInTheDocument();
    expect(within(group()).queryByRole('heading', { name: 'Identity Access' })).not.toBeInTheDocument();
    expect(within(group()).getByTestId('requested-service-status')).toHaveTextContent('Not connected');
    expect(within(group()).getByTestId('requested-service-selection')).toHaveTextContent('No permission set on');
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(saveReason()).toBe('To save: log in to GitHub Production and turn on a permission set that allows it.');
    expect(screen.getByRole('button', { name: 'Save' })).toHaveAccessibleDescription(saveReason());

    await user.click(screen.getByRole('switch', { name: 'Toggle GitHub Issue Triage' }));
    expect(saveReason()).toContain('user:email');
    expect(saveReason()).not.toContain('Connect all required services');
    expect(screen.getByRole('button', { name: 'Save' })).toHaveAccessibleDescription(saveReason());
    await user.click(screen.getByRole('switch', { name: 'Toggle GitHub Read Access' }));
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(saveReason()).toBe('To save: log in to GitHub Production.');
    expect(screen.getByRole('button', { name: 'Save' })).toHaveAccessibleDescription(saveReason());
    expect(within(group()).getByTestId('requested-service-selection')).toHaveTextContent('Permission set on');
    expect(group()).toHaveAttribute('data-state', 'pending');
    expect(screen.getAllByRole('button', { name: /Log in to GitHub Production|^Login$/ })).toHaveLength(1);
    expect(within(group()).getByRole('button', { name: 'Log in to GitHub Production' })).toBeInTheDocument();

    const connectCard = screen.getByRole('article', { name: 'Service: GitHub Production' });
    expect(within(connectCard).getByText('Asked for')).toBeInTheDocument();
    expect(within(connectCard).getByText('Connect above')).toBeInTheDocument();
    expect(within(connectCard).queryByText('Optional')).not.toBeInTheDocument();
  });

  it('enables Save only while every selected candidate contribution is usable', async () => {
    const user = userEvent.setup();
    renderWith({ ...coverage('usable', 'unusable'), required_scopes: [] });
    await user.click(screen.getByRole('switch', { name: 'Toggle GitHub Read Access' }));
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(within(group()).getByTestId('requested-service-status')).toHaveTextContent('Needs reconnecting');
    await user.click(screen.getByRole('switch', { name: 'Toggle GitHub Read Access' }));
    await user.click(screen.getByRole('switch', { name: 'Toggle GitHub Issue Triage' }));
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Save' })).not.toHaveAttribute('aria-describedby');
    expect(group()).toHaveAttribute('data-state', 'ready');
    expect(within(group()).getByTestId('requested-service-status')).toHaveTextContent('Connected');
  });

  it('keeps the connect reason for an unrelated service that still needs a login', () => {
    vi.spyOn(useAgentGrantsModule, 'useAgentGrants').mockReturnValue({
      agent: { ...agent(coverage('missing')), active_session_service_ids: [] },
      services, cimdMeta: null, grants: grant, loading: false, error: null, refetch: vi.fn(),
    } as never);
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/agents/agent-1?session_token=sealed']}>
          <Routes>
            <Route path="*" element={<AgentGrantDetailPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    );
    expect(saveReason()).toBe(
      'Connect all required services before saving. To save: log in to GitHub Production and turn on a permission set that allows it.',
    );
  });

  it('explains that a request no offered set covers cannot be allowed here', () => {
    renderWith({ ...coverage('missing'), covering_permission_sets: [] });
    expect(group()).toHaveTextContent("None of this agent's permission sets include GitHub Production");
    expect(within(group()).queryByRole('switch')).not.toBeInTheDocument();
    expect(saveReason()).toBe('To save: ask an administrator to add a permission set that allows GitHub Production.');
  });
});
