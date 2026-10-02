import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { MemoryRouter } from 'react-router-dom';
import { Bot, CheckCheck, Plug, Search, UserRound } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button/Button';
import { PageHeader } from '../PageHeader/PageHeader';
import { ConsoleShell } from './ConsoleShell';

const meta = {
  title: 'Design System/Layout/ConsoleShell',
  component: ConsoleShell,
  tags: ['autodocs'],
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <MemoryRouter initialEntries={['/delegations']}><Story /></MemoryRouter>],
  beforeEach: () => {
    const previous = localStorage.getItem('aib.sidebar-collapsed');
    localStorage.removeItem('aib.sidebar-collapsed');
    return () => {
      if (previous === null) localStorage.removeItem('aib.sidebar-collapsed');
      else localStorage.setItem('aib.sidebar-collapsed', previous);
    };
  },
  args: {
    labels: {
      wordmark: 'Agentic Identity Broker', skipToMain: 'Skip to main content', navigation: 'Main navigation',
      collapseSidebar: 'Collapse sidebar', expandSidebar: 'Expand sidebar', openNavigation: 'Open navigation',
      closeNavigation: 'Close navigation', navigationDescription: 'Choose a console page.',
      pendingCount: (count) => `${count} pending approvals`, pendingUnknown: 'Pending approvals unavailable',
      pendingStale: 'Last known count; refresh unavailable',
    },
    navigation: [
      { href: '/delegations', label: 'Agents', icon: <Bot className="size-5" /> },
      { href: '/sessions', label: 'Connections', icon: <Plug className="size-5" /> },
      { href: '/approvals', label: 'Approvals', icon: <CheckCheck className="size-5" />, pendingApprovals: true },
    ],
    search: <Button variant="ghost" size="icon" aria-label="Search records"><Search /></Button>,
    userMenu: <Button variant="ghost" size="icon" aria-label="User menu"><UserRound /></Button>,
    pendingCount: 3,
    children: <PageHeader title="Agents" purpose="Manage the access you grant to agents." />,
  },
} satisfies Meta<typeof ConsoleShell>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Desktop: Story = {};
export const Collapsed: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Collapse sidebar' }));
    await expect(canvas.getByRole('img', { name: 'Agentic Identity Broker' })).toHaveAttribute('data-variant', 'compact');
    await expect(canvas.getByRole('link', { name: 'Approvals' })).toHaveAccessibleDescription('3 pending approvals');
  },
};
export const NarrowSheetOpen: Story = {
  globals: { viewport: { value: 'narrow320', isRotated: false } },
  play: async ({ canvasElement }) => {
    const document = canvasElement.ownerDocument;
    await expect(document.defaultView!.innerWidth).toBe(320);
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Open navigation' }));
    const dialog = within(document.body).getByRole('dialog', { name: 'Main navigation' });
    await waitFor(() => expect(dialog).toBeVisible());
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Search records' })).toHaveFocus());
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(320);
    for (const link of within(dialog).getAllByRole('link')) {
      const bounds = link.getBoundingClientRect();
      await expect(bounds.left).toBeGreaterThanOrEqual(0);
      await expect(bounds.right).toBeLessThanOrEqual(320);
    }
  },
};
export const StalePendingCount: Story = { args: { pendingStale: true } };
export const UnknownPendingCount: Story = { args: { pendingCount: undefined } };
export const KeyboardSkip: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await expect(within(canvasElement).getByRole('main')).toHaveFocus();
  },
};
