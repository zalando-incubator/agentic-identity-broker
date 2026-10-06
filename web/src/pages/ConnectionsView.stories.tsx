import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { DisconnectDialogView } from '@components/sessions/DisconnectDialog';
import { deriveConnectionState } from '@components/sessions/connectionState';
import { ConnectionsView } from './ConnectionsView';
import {
  connectedSession as connected, denseSessions, expiredSession as expired,
  longAgentName, needsSignInSession as needsSignIn, sessionByService as byService,
  sessionDetail as details, sessions as sample, storyNow, unavailableSession as unavailable,
} from '../storybook/fixtures';
import { ConsoleStoryShell } from '../storybook/ScreenShell';

async function checkConnectionCards(canvasElement: HTMLElement) {
  for (const card of within(canvasElement).getAllByTestId('connection-card')) {
    const bounds = card.getBoundingClientRect();
    const identity = card.querySelector<HTMLElement>('[data-slot="entity-identity"]')!;
    const title = within(card).getByTestId('entity-name').getBoundingClientRect();
    const badge = card.querySelector<HTMLElement>('[data-slot="entity-status"]')!.getBoundingClientRect();
    const explanation = card.querySelector<HTMLElement>('[data-slot="entity-supporting"]');
    const metadata = card.querySelector<HTMLElement>('[data-slot="entity-meta"]')!;
    const footer = card.querySelector<HTMLElement>('fieldset')!;
    const scope = within(card).getByTestId('connection-scope-count');
    const date = within(card).getByTestId('connection-created-at');
    await expect(identity).toContainElement(within(card).getByTestId('entity-name'));
    await expect(identity).toContainElement(card.querySelector<HTMLElement>('[data-slot="entity-status"]'));
    await expect(badge.top).toBeGreaterThanOrEqual(title.bottom);
    await expect(badge.left).toBeGreaterThanOrEqual(title.left);
    if (explanation) {
      await expect(explanation.getBoundingClientRect().top).toBeGreaterThanOrEqual(badge.bottom);
      await expect(metadata.getBoundingClientRect().top).toBeGreaterThanOrEqual(explanation.getBoundingClientRect().bottom);
    } else await expect(metadata.getBoundingClientRect().top).toBeGreaterThanOrEqual(badge.bottom);
    await expect(metadata).toContainElement(scope);
    await expect(metadata).toContainElement(date);
    await expect(scope).toBeVisible();
    await expect(date).toHaveAttribute('dateTime');
    await expect(date.parentElement!.scrollWidth).toBeLessThanOrEqual(date.parentElement!.clientWidth);
    await expect(footer.getBoundingClientRect().top).toBeGreaterThanOrEqual(metadata.getBoundingClientRect().bottom);
    for (const action of within(footer).getAllByRole('button')) {
      const box = action.getBoundingClientRect();
      await expect(box.left).toBeGreaterThanOrEqual(bounds.left);
      await expect(box.right).toBeLessThanOrEqual(bounds.right);
      await expect(box.bottom).toBeLessThanOrEqual(bounds.bottom);
    }
    await expect(card.scrollHeight).toBeLessThanOrEqual(card.clientHeight);
  }
  await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
}

async function checkConnectionRows(canvasElement: HTMLElement) {
  for (const row of within(canvasElement).getAllByTestId('connection-row')) {
    const bounds = row.getBoundingClientRect();
    const identity = row.querySelector<HTMLElement>('[data-slot="entity-identity"]')!;
    const badge = within(row).getByTestId('connection-state');
    const scope = within(row).getByTestId('connection-scope-count');
    const date = within(row).getByTestId('connection-created-at');
    const disconnect = within(row).getByRole('button', { name: 'Disconnect' });
    const explanation = row.querySelector<HTMLElement>('[data-slot="entity-supporting"]');
    for (const element of [identity, badge, scope, date, disconnect, explanation].filter((value): value is HTMLElement => Boolean(value))) {
      const box = element.getBoundingClientRect();
      await expect(element).toBeVisible();
      await expect(box.left).toBeGreaterThanOrEqual(bounds.left);
      await expect(box.right).toBeLessThanOrEqual(bounds.right);
      await expect(box.bottom).toBeLessThanOrEqual(bounds.bottom);
    }
    await expect(badge.getBoundingClientRect().left).toBeGreaterThanOrEqual(identity.getBoundingClientRect().right);
    await expect(date).toHaveAttribute('dateTime');
    await expect(date.parentElement!.scrollWidth).toBeLessThanOrEqual(date.parentElement!.clientWidth);
    if (explanation) await expect(explanation.scrollHeight).toBeLessThanOrEqual(explanation.clientHeight);
  }
  await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
}

const meta = {
  title: 'Screens/Connections',
  component: ConnectionsView,
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <ConsoleStoryShell path="/connections"><Story /></ConsoleStoryShell>],
  args: {
    sessions: sample, loading: false, error: false, onRetry: fn(),
    getState: (serviceId: string) => deriveConnectionState({ context: 'sessions', session: byService[serviceId], readFailed: serviceId === 'ledger', now: new Date(storyNow) }),
    isRefreshing: () => false, isDisconnecting: () => false, onRefresh: fn(), onDisconnect: fn(),
    search: '', onSearchChange: fn(), sort: 'attention', onSortChange: fn(),
    filter: '', onFilterChange: fn(), view: 'grid', onViewChange: fn(),
  },
} satisfies Meta<typeof ConnectionsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Typical: Story = { play: async ({ canvasElement }) => checkConnectionCards(canvasElement) };
export const Connected: Story = {
  args: { sessions: [connected] },
  play: async ({ canvasElement }) => checkConnectionCards(canvasElement),
};
export const Loading: Story = { args: { sessions: [], loading: true } };
export const Empty: Story = { args: { sessions: [] } };
export const Dense: Story = {
  args: { sessions: denseSessions, getState: () => deriveConnectionState({ context: 'sessions', session: connected }) },
};
export const Error: Story = { args: { sessions: [], error: true } };
export const Stale: Story = { args: { sessions: [unavailable], error: true } };
export const NeedsSignIn: Story = {
  args: { sessions: [needsSignIn] },
  play: async ({ canvasElement }) => {
    await checkConnectionCards(canvasElement);
    const canvas = within(canvasElement);
    const badge = canvas.getByRole('button', { name: 'Needs sign-in' });
    await expect(canvas.queryByText('Your access expired. Refresh to continue.')).not.toBeInTheDocument();
    canvas.getByRole('button', { name: 'List view' }).focus();
    await userEvent.tab();
    await expect(badge).toHaveFocus();
    await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toHaveTextContent('Your access expired. Refresh to continue.');
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).queryByRole('tooltip')).not.toBeInTheDocument());
    await expect(badge).toHaveFocus();
  },
};
export const NeedsSignInTooltip: Story = {
  args: { sessions: [needsSignIn] },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.hover(canvas.getByRole('button', { name: 'Needs sign-in' }));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toHaveTextContent('Your access expired. Refresh to continue.');
  },
};
export const Expired: Story = {
  args: { sessions: [expired] },
  play: async ({ canvasElement }) => checkConnectionCards(canvasElement),
};
export const Unavailable: Story = {
  args: { sessions: [unavailable] },
  play: async ({ canvasElement }) => checkConnectionCards(canvasElement),
};
export const LongProviderName: Story = {
  args: { sessions: [{ ...connected, service_display_name: longAgentName }] },
  play: async ({ canvasElement }) => {
    await checkConnectionCards(canvasElement);
    const card = within(canvasElement).getByTestId('connection-card');
    const name = within(card).getByText(longAgentName);
    await expect(name).toHaveTextContent(longAgentName);
    await waitFor(async () => {
      await userEvent.hover(within(card).getByText(longAgentName));
      await expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent(longAgentName);
    });
  },
};
export const List: Story = { args: { view: 'list' } };
export const UpdatedConnection: Story = { args: { updated: { serviceId: connected.service_id, animationKey: 1 } } };
export const ScopePopover: Story = {
  args: { sessions: [connected] },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const card = canvas.getByTestId('connection-card');
    const height = card.getBoundingClientRect().height;
    await userEvent.click(canvas.getByRole('button', { name: '2 scopes' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByText('mail.read')).toBeVisible());
    await expect(card.getBoundingClientRect().height).toBe(height);
  },
};
export const LongScopeDetails: Story = {
  args: { sessions: [{ ...connected, scope: ['https://permissions.example.com/organisations/regulatory-evidence/archive/records.read', 'mail.read'] }] },
  play: async ({ canvasElement, args }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(within(canvasElement).getByRole('button', { name: '2 scopes' }));
    const details = await body.findByRole('dialog', { name: 'Granted scopes' });
    await waitFor(() => expect(details).toBeVisible());
    for (const scope of args.sessions[0]!.scope) {
      const value = within(details).getByText(scope, { exact: true });
      await expect(value).toBeVisible();
      await expect(value.scrollWidth).toBeLessThanOrEqual(value.clientWidth);
    }
    const bounds = details.getBoundingClientRect();
    await expect(bounds.left).toBeGreaterThanOrEqual(0);
    await expect(bounds.right).toBeLessThanOrEqual(window.innerWidth);
  },
};
export const EmptyScopes: Story = {
  args: { sessions: [{ ...connected, scope: [] }] },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByTestId('connection-scope-count')).toHaveTextContent('0 scopes');
    await expect(canvas.queryByRole('button', { name: '0 scopes' })).not.toBeInTheDocument();
    await expect(canvas.getByTestId('connection-created-at')).toBeVisible();
  },
};
export const DisconnectConfirmation: Story = {
  args: {
    sessions: [connected],
    dialog: <DisconnectDialogView session={connected} affectedAgents={details.dependent_agents.map(agent => ({ agent_id: agent.id, display_name: agent.display_name }))} loading={false} error={false}
      onRetry={fn()} onCancel={fn()} onConfirm={fn()} />,
  },
};

export const CompactCards: Story = {
  play: async ({ canvasElement }) => checkConnectionCards(canvasElement),
};
export const LongProviderCard: Story = {
  args: { sessions: [{ ...connected, service_display_name: longAgentName }] },
  play: async ({ canvasElement }) => checkConnectionCards(canvasElement),
};
export const ResponsiveRows: Story = {
  args: { view: 'list' },
  play: async ({ canvasElement, args }) => {
    await checkConnectionRows(canvasElement);
    const connectedRow = within(canvasElement).getAllByTestId('connection-row').find(row => row.id === `connection-${connected.id}`)!;
    const scope = within(connectedRow).getByRole('button', { name: '2 scopes' });
    scope.focus();
    await expect(scope).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    const body = within(canvasElement.ownerDocument.body);
    const details = await body.findByRole('dialog', { name: 'Granted scopes' });
    await waitFor(() => expect(within(details).getByText('mail.read')).toBeVisible());
    await waitFor(() => expect(details).toHaveFocus());
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(details).not.toBeInTheDocument());
    await waitFor(() => expect(scope).toHaveFocus());
    await userEvent.tab();
    const disconnect = within(connectedRow).getByRole('button', { name: 'Disconnect' });
    await expect(disconnect).toHaveFocus();
    await userEvent.click(disconnect);
    await expect(args.onDisconnect).toHaveBeenCalledWith(connected);
    await checkConnectionRows(canvasElement);
  },
};
