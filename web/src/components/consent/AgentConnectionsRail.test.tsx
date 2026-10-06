import { MemoryRouter } from 'react-router-dom';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { deriveConnectionState, transitionConnectionState } from '@components/sessions/connectionState';
import { AgentConnectionsRail, type AgentConnection } from './AgentConnectionsRail';
import type { SessionSummary } from '@services/api/sessions';

it('exposes full provider names and preserves Connect and Manage actions', async () => {
  const connect = vi.fn();
  const longName = 'International records and regulatory evidence preservation provider';
  const connections: AgentConnection[] = [
    { service: { kind: 'requirement', serviceId: 'long-service', serviceName: longName, requirementType: 'mandatory', connectionStatus: 'not_connected', requiredScopes: [] }, state: deriveConnectionState({ context: 'requirement', connectionStatus: 'not_connected' }) },
    { service: { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'optional', connectionStatus: 'connected', requiredScopes: [] }, state: deriveConnectionState({ context: 'requirement', connectionStatus: 'connected' }) },
  ];
  render(<MemoryRouter><AgentConnectionsRail connections={connections} loading={false} error={false} storageError={false} onRetry={vi.fn()} onConnect={connect} /></MemoryRouter>);

  const [missing, connected] = screen.getAllByTestId('agent-connection-row');
  expect(within(missing!).getByTestId('connection-provider')).toHaveTextContent(longName);
  expect(within(missing!).getByTestId('connection-state')).toHaveTextContent('No connection');
  await userEvent.click(within(missing!).getByRole('button', { name: `Connect ${longName}`, exact: true }));
  expect(connect).toHaveBeenCalledExactlyOnceWith('long-service');
  expect(within(connected!).getByTestId('connection-state')).toHaveTextContent('Connected');
  expect(within(connected!).getByRole('link', { name: 'Manage connection' })).toHaveAttribute('href', '/connections');
  expect(within(connected!).getByRole('link', { name: 'Manage connection' })).toHaveTextContent('Manage');
});

const now = new Date('2026-09-27T12:00:00Z');
const session: SessionSummary = {
  id: 'mail-session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer', scope: ['read'],
  initiated_at: '2026-09-01T00:00:00Z', is_expired: false, access_token_expired: false,
  has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};
const connected = deriveConnectionState({ context: 'sessions', session, now });

it.each([
  { label: 'Connected', state: connected, explanation: 'This service is connected.' },
  { label: 'No connection', state: deriveConnectionState({ context: 'requirement', connectionStatus: 'not_connected', now }), explanation: 'No connection yet. Connect this service before using it with this agent.' },
  { label: 'Expired', state: deriveConnectionState({ context: 'sessions', session: { ...session, is_expired: true }, now }), explanation: 'Your access expired. Reconnect to continue.' },
  { label: 'Needs sign-in', state: transitionConnectionState(connected, { type: 'refresh-failed', status: 409 }), explanation: 'Sign in again to continue.' },
  { label: 'Unavailable', state: deriveConnectionState({ context: 'sessions', now }), explanation: 'Connection details are unavailable. Try again.' },
  { label: 'Unavailable', state: transitionConnectionState(connected, { type: 'read-failed' }), explanation: 'Connection details may be out of date. Open Connections to check the latest status.' },
])('explains $label on hover and focus', async ({ label, state, explanation }) => {
  const user = userEvent.setup();
  const service: AgentConnection['service'] = { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', connectionStatus: 'not_connected', requiredScopes: [] };
  render(<MemoryRouter><AgentConnectionsRail connections={[{ service, state }]} loading={false} error={false} storageError={false} onRetry={vi.fn()} onConnect={vi.fn()} /></MemoryRouter>);
  const badge = screen.getByTestId('connection-state');
  expect(badge).toHaveTextContent(label);
  const trigger = badge.parentElement!;
  await user.hover(trigger);
  expect(await screen.findByRole('tooltip')).toHaveTextContent(explanation);
  expect(trigger).toHaveAccessibleDescription(explanation);
  await user.unhover(trigger);
  await user.pointer({ target: document.body, coords: { clientX: 1000, clientY: 1000 } });
  await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  await user.tab();
  expect(trigger).toHaveFocus();
  expect(await screen.findByRole('tooltip')).toHaveTextContent(explanation);
  expect(trigger).toHaveAccessibleDescription(explanation);
});

it('keeps a focused badge explanation through ancestor scrolling and dismisses it on Escape or blur', async () => {
  const user = userEvent.setup();
  const service: AgentConnection['service'] = { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', connectionStatus: 'connected', requiredScopes: [] };
  render(<MemoryRouter><AgentConnectionsRail connections={[{ service, state: connected }]} loading={false} error={false} storageError={false} onRetry={vi.fn()} onConnect={vi.fn()} /></MemoryRouter>);
  const trigger = screen.getByTestId('connection-state').parentElement!;
  await user.tab();
  expect(await screen.findByRole('tooltip')).toHaveTextContent('This service is connected.');
  fireEvent.scroll(document.body);
  expect(trigger).toHaveAccessibleDescription('This service is connected.');
  await user.keyboard('{Escape}');
  await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
  await user.tab();
  await user.keyboard('{Shift>}{Tab}{/Shift}');
  expect(await screen.findByRole('tooltip')).toHaveTextContent('This service is connected.');
  await user.tab();
  await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  expect(screen.getByRole('link', { name: 'Manage connection' })).toHaveFocus();
});

it('keeps the badge informational while the separate Connect action remains keyboard-operable', async () => {
  const user = userEvent.setup();
  const onConnect = vi.fn();
  const service: AgentConnection['service'] = { kind: 'requirement', serviceId: 'mail', serviceName: 'Mail', requirementType: 'mandatory', connectionStatus: 'not_connected', requiredScopes: [] };
  render(<MemoryRouter><AgentConnectionsRail connections={[{ service, state: deriveConnectionState({ context: 'requirement', connectionStatus: 'not_connected', now }) }]} loading={false} error={false} storageError={false} onRetry={vi.fn()} onConnect={onConnect} /></MemoryRouter>);
  await user.tab();
  expect(screen.getByTestId('connection-state').parentElement).toHaveFocus();
  await user.keyboard('{Enter}');
  expect(onConnect).not.toHaveBeenCalled();
  await user.tab();
  expect(screen.getByRole('button', { name: 'Connect Mail', exact: true })).toHaveFocus();
  await user.keyboard('{Enter}');
  expect(onConnect).toHaveBeenCalledExactlyOnceWith('mail');
});
