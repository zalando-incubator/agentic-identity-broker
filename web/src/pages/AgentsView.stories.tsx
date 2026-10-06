import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import type { AgentDelegation } from '@app-types/consent';
import { agents, denseAgents, longAgent, storyNow } from '../storybook/fixtures';
import { ConsoleStoryShell } from '../storybook/ScreenShell';
import { AgentsView, type AgentsViewProps } from './AgentsView';

function AgentsDemo(args: AgentsViewProps) {
  const [search, setSearch] = useState(args.search);
  const [sort, setSort] = useState(args.sort);
  const [view, setView] = useState(args.view);
  const [confirmation, setConfirmation] = useState<AgentDelegation | null>(args.confirmation);
  return <AgentsView {...args} search={search} onSearchChange={setSearch} sort={sort} onSortChange={setSort}
    view={view} onViewChange={setView} confirmation={confirmation} onRevoke={setConfirmation} onCancelRevoke={() => setConfirmation(null)} />;
}

async function checkAgentRows(canvasElement: HTMLElement) {
  const rows = within(canvasElement).getAllByTestId('agent-entity');
  for (const row of rows) {
    const bounds = row.getBoundingClientRect();
    const identity = row.querySelector('[data-slot="entity-identity"]')!.getBoundingClientRect();
    const expiry = within(row).getByTestId('agent-expiry').getBoundingClientRect();
    const date = within(row).getByTestId('agent-changed-at');
    const dateSlot = row.querySelector('[data-slot="entity-meta"]')!;
    const action = within(row).getByRole('button', { name: 'Revoke' }).getBoundingClientRect();
    await expect(action.right).toBeLessThanOrEqual(bounds.right);
    await expect(date).toHaveAttribute('dateTime');
    if (window.innerWidth >= 1280) {
      await expect(expiry.left).toBeGreaterThanOrEqual(identity.right);
      await expect(dateSlot.getBoundingClientRect().left).toBeGreaterThanOrEqual(expiry.right);
    }
  }
  await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
}

const meta = {
  title: 'Screens/Agents/Collection',
  component: AgentsView,
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <ConsoleStoryShell path="/agents"><Story /></ConsoleStoryShell>],
  render: args => <AgentsDemo {...args} />,
  args: {
    delegations: agents,
    loading: false,
    error: false,
    onRetry: fn(),
    search: '', onSearchChange: fn(),
    sort: 'name', onSortChange: fn(),
    view: 'grid', onViewChange: fn(),
    onRevoke: fn(), isPending: () => false,
    confirmation: null, onCancelRevoke: fn(), onConfirmRevoke: fn(),
    now: storyNow,
  },
} satisfies Meta<typeof AgentsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Typical: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const entities = canvas.getAllByTestId('agent-entity');
    await expect(entities).toHaveLength(3);
    await expect(within(entities[0]!).getByRole('link', { name: 'Calendar assistant' })).toHaveAttribute('href', '/agents/calendar');
    await expect(within(entities[0]!).getByRole('button', { name: 'Revoke' })).toBeVisible();
    const height = canvas.getByTestId('agents-collection').getBoundingClientRect().height;
    await userEvent.click(canvas.getByRole('button', { name: 'Sort' }));
    await expect(canvas.getByTestId('agents-collection').getBoundingClientRect().height).toBe(height);
  },
};
export const Loading: Story = { args: { delegations: [], loading: true } };
export const Empty: Story = { args: { delegations: [] } };
export const Dense: Story = { args: { delegations: denseAgents, view: 'list' } };
export const Error: Story = { args: { delegations: [], error: true } };
export const Stale: Story = { args: { error: true } };
export const NoMatches: Story = { args: { search: 'No matching agent' } };
export const LongName: Story = {
  args: { delegations: [longAgent, ...agents] },
  play: async ({ canvasElement }) => {
    const link = within(canvasElement).getByRole('link', { name: longAgent.displayName });
    await userEvent.hover(link);
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent(longAgent.displayName));
  },
};
export const ConfirmRevoke: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(within(canvas.getAllByTestId('agent-entity')[0]!).getByRole('button', { name: 'Revoke' }));
    const dialog = within(canvasElement.ownerDocument.body).getByRole('dialog', { name: 'Revoke access' });
    await expect(dialog).toHaveTextContent('Calendar assistant');
    await expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Revoke' }));
    await expect(args.onConfirmRevoke).toHaveBeenCalledOnce();
  },
};

export const CompactCards: Story = {
  args: { delegations: [longAgent, ...agents] },
  play: async ({ canvasElement }) => {
    const cards = within(canvasElement).getAllByTestId('agent-entity');
    for (const card of cards) {
      const bounds = card.getBoundingClientRect();
      const meta = card.querySelector('[data-slot="entity-meta"]')!.getBoundingClientRect();
      const action = within(card).getByRole('button', { name: 'Revoke' }).getBoundingClientRect();
      await expect(action.top).toBeGreaterThanOrEqual(meta.bottom);
      await expect(action.right).toBeLessThanOrEqual(bounds.right);
      await expect(action.bottom).toBeLessThanOrEqual(bounds.bottom);
      await expect(within(card).getByTestId('agent-expiry')).toBeVisible();
      await expect(within(card).getByTestId('agent-changed-at')).toHaveAttribute('dateTime');
    }
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  },
};

export const PendingCards: Story = {
  args: { delegations: [longAgent, ...agents], isPending: (id: string) => id === 'research' },
  play: async ({ canvasElement }) => {
    const cards = within(canvasElement).getAllByTestId('agent-entity');
    const pending = cards.find(card => card.getAttribute('aria-busy') === 'true')!;
    await expect(within(pending).getByRole('button', { name: 'Revoke' })).toBeDisabled();
    await expect(within(cards[0]!).getByTestId('agent-expiry')).toHaveTextContent('Expires soon');
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  },
};

export const Rows: Story = {
  args: { view: 'list', delegations: [longAgent, ...agents] },
  play: async ({ canvasElement }) => {
    await checkAgentRows(canvasElement);
    const row = within(canvasElement).getAllByTestId('agent-entity')[0]!;
    const link = within(row).getByRole('link', { name: 'Calendar assistant' });
    link.focus();
    await expect(link).toHaveFocus();
    await userEvent.tab();
    const revoke = within(row).getByRole('button', { name: 'Revoke' });
    await expect(revoke).toHaveFocus();
    await userEvent.click(revoke);
    const dialog = within(canvasElement.ownerDocument.body).getByRole('dialog', { name: 'Revoke access' });
    await expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await expect(link).toHaveAttribute('href', '/agents/calendar');
  },
};

export const PendingRows: Story = {
  args: { view: 'list', delegations: [longAgent, ...agents], isPending: (id: string) => id === 'research' },
  play: async ({ canvasElement }) => {
    const rows = within(canvasElement).getAllByTestId('agent-entity');
    const pending = rows.find(row => row.getAttribute('aria-busy') === 'true')!;
    await expect(within(pending).getByRole('button', { name: 'Revoke' })).toBeDisabled();
    await expect(within(rows[0]!).getByTestId('agent-expiry')).toHaveTextContent('Expires soon');
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  },
};
