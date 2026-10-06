import { useState, type FormEvent } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { createConsentDraft } from '@components/consent/consentDraft';
import type { AgentConnection } from '@components/consent/AgentConnectionsRail';
import { deriveConnectionState } from '@components/sessions/connectionState';
import { AgentConsoleView, type AgentConsoleViewProps } from './AgentConsoleView';
import { consentDetail, consentDenseDetail, consentEmptyDetail, consentGrant, longAgentName, longDescription, needsSignInSession } from '../storybook/fixtures';
import { ConsoleStoryShell } from '../storybook/ScreenShell';

const draft = createConsentDraft({ permissionSets: consentDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, existingGrant: consentGrant, context: 'console' });
const connections: AgentConnection[] = consentDetail.services.map(service => ({ service, state: deriveConnectionState({ context: 'requirement', connectionStatus: service.connectionStatus }) }));

function EditorDemo(args: AgentConsoleViewProps) {
  if (args.state !== 'ready') return <AgentConsoleView {...args} />;
  return <ReadyEditorDemo {...args} />;
}
function ReadyEditorDemo(args: Extract<AgentConsoleViewProps, { state: 'ready' }>) {
  const [draftState, setDraft] = useState(args.draft);
  const [confirmation, setConfirmation] = useState(args.revokeConfirmation);
  return <AgentConsoleView {...args} draft={draftState} onDraftChange={setDraft} onDurationChange={setDraft}
    onCancel={() => setDraft(args.draft)} onRevoke={() => setConfirmation(true)} revokeConfirmation={confirmation}
    onCancelRevoke={() => setConfirmation(false)} />;
}

const meta = {
  title: 'Screens/Agents/Detail',
  component: AgentConsoleView,
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <ConsoleStoryShell path="/agents/research-agent"><Story /></ConsoleStoryShell>],
  render: args => <EditorDemo {...args} />,
  args: {
    state: 'ready', data: consentDetail, draft, savedGrant: consentGrant,
    onRetry: fn(), pending: false, blocked: false,
    onSubmit: fn((event: FormEvent<HTMLFormElement>) => { event.preventDefault(); }), onDraftChange: fn(), onDurationChange: fn(), onCancel: fn(),
    connections, connectionLoading: false, connectionError: false, connectionStorageError: false,
    onConnectionRetry: fn(), onConnect: fn(),
    onRevoke: fn(), revokeConfirmation: false, revokePending: false,
    onCancelRevoke: fn(), onConfirmRevoke: fn(), onReturnRevokeFocus: fn(),
  },
} satisfies Meta<typeof AgentConsoleView>;
export default meta;
type Story = StoryObj<AgentConsoleViewProps>;

export const Typical: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole('tab')).not.toBeInTheDocument();
    await expect(canvas.getByTestId('agent-connections')).toBeVisible();
    const duration = canvas.getByRole('combobox', { name: 'Access lasts' });
    await expect(duration).toBeVisible();
    await userEvent.click(duration);
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('listbox')).toBeVisible(), { timeout: 5_000 });
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(canvas.getByRole('combobox', { name: 'Access lasts' })).toHaveFocus());
    await userEvent.click(canvas.getByRole('checkbox', { name: 'Access group 2' }));
    await expect(canvas.getByTestId('grant-save-bar')).toBeVisible();
    await userEvent.click(canvas.getByRole('button', { name: 'Cancel' }));
    await expect(canvas.queryByTestId('grant-save-bar')).not.toBeInTheDocument();
  },
};
export const TechnicalDetails: Story = {
  args: { data: { ...consentDetail, agent: { ...consentDetail.agent,
    clientId: 'https://agents.example.org/clients/research-agent/metadata.json',
    clientUris: [
      'https://agents.example.org/products/research-agent?tenant=operations',
      'https://agents.example.org/agents/research-agent/technical-information-with-a-very-long-unbroken-segment',
    ],
  } } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const technical = canvas.getByRole('region', { name: 'Technical details' });
    await expect(within(technical).getByText('research-agent')).toBeVisible();
    await expect(within(technical).getByText('https://agents.example.org/clients/research-agent/metadata.json')).toBeVisible();
    await expect(within(technical).getByText('https://agents.example.org/products/research-agent?tenant=operations')).toBeVisible();
    await expect(within(technical).getByText('https://agents.example.org/agents/research-agent/technical-information-with-a-very-long-unbroken-segment')).toBeVisible();
    await expect(within(technical).getByText(consentGrant.id)).toBeVisible();
    await expect(within(technical).queryByText('Redirect URI')).not.toBeInTheDocument();
    await expect(canvas.getByTestId('agent-name-heading').closest('header')).not.toHaveTextContent('agents.example.org');
    await expect(within(canvas.getByTestId('agent-about')).getByRole('link', { name: 'Governance' })).toBeVisible();
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  },
};
export const Loading: Story = { args: { state: 'loading' } };
export const Error: Story = { args: { state: 'error' } };
export const Stale: Story = { args: { stale: true } };
export const Empty: Story = {
  args: { data: consentEmptyDetail, savedGrant: null, draft: createConsentDraft({ permissionSets: [], serviceRequirements: [], existingGrant: null, context: 'console' }), connections: [] },
  play: async ({ canvasElement }) => {
    const technical = within(canvasElement).getByRole('region', { name: 'Technical details' });
    await expect(within(technical).getByText('research-agent')).toBeVisible();
    for (const label of ['Client ID', 'Registered client URIs', 'Grant ID', 'Grant created', 'Grant updated', 'Redirect URI', 'Requested scopes']) {
      await expect(within(technical).queryByText(label)).not.toBeInTheDocument();
    }
  },
};
export const Dense: Story = { args: { data: consentDenseDetail, draft: createConsentDraft({ permissionSets: consentDenseDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, existingGrant: consentGrant, context: 'console' }) } };
export const LongContent: Story = { args: { data: { ...consentDetail, agent: { ...consentDetail.agent, displayName: longAgentName, description: longDescription } } } };
export const SaveFailure: Story = { args: { error: 'Your changes were not saved. Your access has not changed.' } };
export const NoConfiguredAboutLinks: Story = {
  args: { data: { ...consentDetail, agent: { ...consentDetail.agent, governanceUrl: undefined, userDocumentationUrl: undefined, agentInterfaceUrl: undefined }, cimd_metadata: undefined } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const about = canvas.getByTestId('agent-about');
    await expect(within(about).getByText(consentDetail.agent.description!)).toBeVisible();
    await expect(within(about).queryByRole('link')).not.toBeInTheDocument();
    await expect(within(canvas.getByTestId('agent-name-heading').closest('header')!).queryByText(consentDetail.agent.description!)).not.toBeInTheDocument();
  },
};
export const WithoutAbout: Story = { args: { data: { ...consentDetail, agent: { ...consentDetail.agent, description: '', governanceUrl: undefined, userDocumentationUrl: undefined, agentInterfaceUrl: undefined } } } };
export const UntilRevoked: Story = {
  args: {
    savedGrant: { ...consentGrant, valid_until: undefined },
    draft: createConsentDraft({ permissionSets: consentDetail.agent.permission_sets, serviceRequirements: consentDetail.agent.service_requirements, existingGrant: { ...consentGrant, valid_until: undefined }, context: 'console' }),
  },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('Until I revoke it');
  },
};
export const DurationSettings: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const original = canvas.getByRole('combobox', { name: 'Access lasts' }).textContent;
    await userEvent.click(canvas.getByRole('combobox', { name: 'Access lasts' }));
    await userEvent.click(await within(canvasElement.ownerDocument.body).findByRole('option', { name: 'Until I revoke it' }));
    await expect(canvas.getByRole('combobox', { name: 'Access lasts' })).toHaveTextContent('Until I revoke it');
    await userEvent.click(canvas.getByRole('button', { name: 'Cancel' }));
    await expect(canvas.getByRole('combobox', { name: 'Access lasts' }).textContent).toBe(original);
  },
};
export const DateValidation: Story = {
  args: { draft: draft.setDuration('custom', '2000-01-01'), dateError: 'Choose a date after today.' },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole('combobox', { name: 'Access lasts' });
    await expect(trigger).toHaveAttribute('aria-invalid', 'true');
    await expect(await within(canvasElement.ownerDocument.body).findByLabelText('Custom date', { selector: 'input' })).toHaveValue('2000-01-01');
  },
};
export const LongProvider: Story = {
  args: {
    data: { ...consentDetail, agent: { ...consentDetail.agent, active_session_service_ids: ['drive'] }, services: [
      { ...consentDetail.services[0]!, serviceName: 'International records and regulatory evidence preservation provider', connectionStatus: 'not_connected' },
      ...consentDetail.services.slice(1),
    ] },
    connections: [
      { service: { ...consentDetail.services[0]!, serviceName: 'International records and regulatory evidence preservation provider', connectionStatus: 'not_connected' }, state: deriveConnectionState({ context: 'requirement', connectionStatus: 'not_connected' }) },
      { service: consentDetail.services[1]!, state: deriveConnectionState({ context: 'sessions', session: { ...needsSignInSession, id: 'drive-connection', service_id: 'drive', service_display_name: 'Drive' } }) },
    ],
    blocked: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const first = canvas.getAllByTestId('agent-connection-row')[0]!;
    const second = canvas.getAllByTestId('agent-connection-row')[1]!;
    await expect(within(first).getByTestId('connection-provider').getBoundingClientRect().bottom).toBeLessThan(within(first).getByTestId('connection-state').getBoundingClientRect().top);
    await expect(within(first).getByRole('button', { name: /^Connect / })).toBeVisible();
    await expect(within(second).getByTestId('connection-state')).toHaveTextContent('Needs sign-in');
    await expect(within(second).getByRole('button', { name: 'Reconnect Drive', exact: true })).toBeVisible();
    const status = within(second).getByTestId('connection-state').parentElement!;
    status.focus();
    await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toHaveTextContent('Sign in again to continue.');
    await waitFor(() => expect(status).toHaveAccessibleDescription('Sign in again to continue.'));
    await userEvent.keyboard('{Escape}');
  },
};
export const ConfirmRevoke: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Revoke access' }));
    const dialog = within(canvasElement.ownerDocument.body).getByRole('dialog', { name: 'Revoke access' });
    await expect(dialog).toHaveTextContent('Research assistant');
    await expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  },
};
