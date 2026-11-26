import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RequestedServiceGroup } from './RequestedServiceGroup';
import type { RequestedServiceResult } from './requestedServiceGate';

const result = (overrides: Partial<RequestedServiceResult> = {}): RequestedServiceResult => ({
  serviceId: 'gh',
  serviceName: 'GitHub Production',
  satisfied: false,
  login: 'missing',
  selection: 'none',
  missingScopes: [],
  reason: 'To save: log in to GitHub Production and turn on a permission set that allows it.',
  ...overrides,
});

const renderGroup = (overrides: Partial<React.ComponentProps<typeof RequestedServiceGroup>> = {}) => {
  const props: React.ComponentProps<typeof RequestedServiceGroup> = {
    agentName: 'Data Analysis Assistant',
    result: result(),
    hasCoveringSets: true,
    alsoAllowedBy: [],
    onRetry: vi.fn(),
    children: [<div key="a">GitHub Read Access row</div>, <div key="b">GitHub Issue Triage row</div>],
    ...overrides,
  };
  render(<RequestedServiceGroup {...props} />);
  return props;
};

describe('RequestedServiceGroup', () => {
  it('labels the group by its header and nests every covering row under it', () => {
    renderGroup();
    const group = screen.getByRole('group', { name: 'Data Analysis Assistant is asking for GitHub Production' });
    expect(within(group).getByText('Turn on at least one of these permission sets to allow it.')).toBeInTheDocument();
    expect(within(group).getByText('GitHub Read Access row')).toBeInTheDocument();
    expect(within(group).getByText('GitHub Issue Triage row')).toBeInTheDocument();
    expect(within(group).getAllByTestId('requested-service-row')).toHaveLength(2);
    expect(group).toHaveAttribute('data-state', 'pending');
    expect(group).not.toHaveTextContent('Session:');
  });

  it('states both unmet conditions and points to the usual login without adding a button', () => {
    renderGroup();
    expect(screen.getByTestId('requested-service-status')).toHaveTextContent('Not connected');
    expect(screen.getByTestId('requested-service-selection')).toHaveTextContent('No permission set on');
    expect(screen.getByTestId('requested-service-login-hint')).toHaveTextContent(
      'Log in to GitHub Production under Connect Your Accounts below.',
    );
    expect(screen.queryByRole('button', { name: /Log in/ })).not.toBeInTheDocument();
  });

  it.each([
    ['usable', 'Connected'],
    ['missing', 'Not connected'],
    ['unusable', 'Needs reconnecting'],
    ['unavailable', "Can't check right now"],
  ] as const)('maps %s usability to %s', (login, label) => {
    renderGroup({ result: result({ login }) });
    expect(screen.getByTestId('requested-service-status')).toHaveTextContent(label);
    const hint = screen.queryByTestId('requested-service-login-hint');
    if (login === 'missing' || login === 'unusable') {
      expect(hint).toBeInTheDocument();
    } else {
      expect(hint).not.toBeInTheDocument();
    }
    expect(screen.queryByRole('button', { name: /Log in/ })).not.toBeInTheDocument();
  });

  it('offers Retry while usability cannot be checked', async () => {
    const { onRetry } = renderGroup({ result: result({ login: 'unavailable' }) });
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalled();
  });

  it('turns to the success state once connected with a covering set on', () => {
    renderGroup({ result: result({ login: 'usable', selection: 'on', satisfied: true, reason: '' }) });
    const group = screen.getByRole('group');
    expect(group).toHaveAttribute('data-state', 'ready');
    expect(screen.getByTestId('requested-service-status')).toHaveTextContent('Connected');
    expect(screen.getByTestId('requested-service-selection')).toHaveTextContent('Permission set on');
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('says more sets are needed when the selection lacks required scopes', () => {
    renderGroup({ result: result({ login: 'usable', selection: 'incomplete', missingScopes: ['user:email'] }) });
    expect(screen.getByTestId('requested-service-selection')).toHaveTextContent('More permission sets needed');
    expect(screen.getByRole('group')).toHaveAttribute('data-state', 'pending');
  });

  it('names sets shown under an earlier request that also allow this service', () => {
    renderGroup({ alsoAllowedBy: ['GitHub Read Access'] });
    expect(screen.getByText('Also allowed by GitHub Read Access, shown above.')).toBeInTheDocument();
  });

  it('explains that the service cannot be allowed when no set covers it', () => {
    renderGroup({ hasCoveringSets: false, children: [] });
    const group = screen.getByRole('group', { name: 'Data Analysis Assistant is asking for GitHub Production' });
    expect(group).toHaveTextContent(
      "None of this agent's permission sets include GitHub Production, so you can't allow it here. An administrator can add one.",
    );
    expect(screen.queryByTestId('requested-service-row')).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(screen.queryByText('Turn on at least one of these permission sets to allow it.')).not.toBeInTheDocument();
  });
});
