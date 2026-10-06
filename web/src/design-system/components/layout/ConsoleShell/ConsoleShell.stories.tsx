import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { MemoryRouter } from 'react-router-dom';
import { Bot, CheckCheck, Plug, Search, UserRound } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button/Button';
import { PageHeader } from '../PageHeader/PageHeader';
import { ConsoleShell, type ConsoleShellProps } from './ConsoleShell';

const meta = {
  title: 'Design System/Layout/ConsoleShell',
  component: ConsoleShell,
  tags: ['autodocs'],
  parameters: { layout: 'fullscreen', a11y: { test: 'error' } },
  decorators: [(Story) => <MemoryRouter initialEntries={['/agents']}><Story /></MemoryRouter>],
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
      { href: '/agents', label: 'Agents', icon: <Bot className="size-5" /> },
      { href: '/connections', label: 'Connections', icon: <Plug className="size-5" /> },
      { href: '/approvals', label: 'Approvals', icon: <CheckCheck className="size-5" />, pendingApprovals: true },
    ],
    search: <Button variant="ghost" className="w-full justify-start px-2 text-muted-foreground group-data-[state=collapsed]/sidebar:justify-center" aria-label="Search records"><Search aria-hidden="true" /><span className="group-data-[state=collapsed]/sidebar:hidden">Search</span></Button>,
    userMenu: <Button variant="ghost" className="w-full justify-start" aria-label="User menu"><UserRound aria-hidden="true" /> <span className="group-data-[state=collapsed]/sidebar:sr-only">User menu</span></Button>,
    pendingCount: 3,
    children: <PageHeader title="Agents" count={4} />,
  },
} satisfies Meta<typeof ConsoleShell>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Desktop: Story = {};
export const LongPage: Story = {
  args: { children: <div className="min-h-[1800px]"><PageHeader title="Long collection" /><p>Collection content continues below the viewport.</p></div> },
  play: async ({ canvasElement }) => {
    if (window.innerWidth < 1012) return;
    const canvas = within(canvasElement);
    const sidebar = canvas.getByTestId('console-sidebar');
    const main = canvas.getByRole('main');
    expect(sidebar.getBoundingClientRect().height).toBe(main.getBoundingClientRect().height);
    const nav = canvas.getByRole('link', { name: 'Agents' });
    const menu = canvas.getByRole('button', { name: 'User menu' });
    const navTop = nav.getBoundingClientRect().top;
    const menuBottom = menu.getBoundingClientRect().bottom;
    window.scrollTo(0, 600);
    await waitFor(() => expect(window.scrollY).toBe(600));
    expect(nav.getBoundingClientRect().top).toBe(navTop);
    expect(menu.getBoundingClientRect().bottom).toBe(menuBottom);
    expect(menuBottom).toBeLessThanOrEqual(window.innerHeight);
    window.scrollTo(0, 0);
  },
};
export const Collapsed: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    if (window.innerWidth < 1012) {
      await expect(canvas.getByRole('button', { name: 'Open navigation' })).toBeVisible();
      return;
    }
    await userEvent.click(canvas.getByRole('button', { name: 'Collapse sidebar' }));
    await expect(canvas.getByRole('img', { name: 'Agentic Identity Broker' })).toHaveAttribute('data-variant', 'compact');
    await expect(canvas.getByRole('link', { name: 'Approvals' })).toHaveAccessibleDescription('3 pending approvals');
  },
};
export const CollapsedHover: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    if (window.innerWidth < 1012) {
      await expect(canvas.getByRole('button', { name: 'Open navigation' })).toBeVisible();
      return;
    }
    await userEvent.click(canvas.getByRole('button', { name: 'Collapse sidebar' }));
    await userEvent.hover(canvas.getByRole('button', { name: 'Expand sidebar' }));
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
export const TabletSheetOpen: Story = {
  globals: { viewport: { value: 'tablet', isRotated: false } },
  play: async ({ canvasElement }) => {
    const document = canvasElement.ownerDocument;
    await expect(document.defaultView!.innerWidth).toBe(768);
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Open navigation' }));
    await waitFor(() => expect(within(document.body).getByRole('dialog', { name: 'Main navigation' })).toBeVisible());
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(768);
  },
};
export const StalePendingCount: Story = {
  args: { pendingStale: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    if (window.innerWidth < 1012) await userEvent.click(canvas.getByRole('button', { name: 'Open navigation' }));
    await userEvent.hover(within(canvasElement.ownerDocument.body).getByRole('button', { name: 'Last known count; refresh unavailable' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent('Last known count; refresh unavailable'));
  },
};
export const ZeroPendingCount: Story = {
  args: { pendingCount: 0 },
  play: async ({ canvasElement }) => {
    if (window.innerWidth < 1012) await userEvent.click(within(canvasElement).getByRole('button', { name: 'Open navigation' }));
    const body = within(canvasElement.ownerDocument.body);
    await expect(body.queryByTestId('pending-approval-count')).not.toBeInTheDocument();
    await expect(body.getByRole('link', { name: 'Approvals' })).toHaveAccessibleDescription('0 pending approvals');
  },
};
export const UnknownPendingCount: Story = { args: { pendingCount: undefined } };

function StaleShell({ children, ...shellProps }: ConsoleShellProps) {
  const [stale, setStale] = useState(false);
  return <ConsoleShell {...shellProps} pendingStale={stale}>
    {children}
    <Button onClick={() => setStale(true)}>Mark count stale</Button>
  </ConsoleShell>;
}
export const StaleLayoutStability: Story = {
  render: (args) => <StaleShell {...args} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    if (window.innerWidth < 1012) {
      const nav = canvas.getByRole('button', { name: 'Open navigation' });
      const before = nav.getBoundingClientRect();
      await userEvent.click(canvas.getByRole('button', { name: 'Mark count stale' }));
      const after = nav.getBoundingClientRect();
      await expect([after.x, after.y, after.width, after.height]).toEqual([before.x, before.y, before.width, before.height]);
      return;
    }
    const approvals = canvas.getByRole('link', { name: 'Approvals' });
    const before = approvals.getBoundingClientRect();
    await userEvent.click(canvas.getByRole('button', { name: 'Mark count stale' }));
    const after = approvals.getBoundingClientRect();
    await expect(after.top).toBe(before.top);
    await expect(after.height).toBe(before.height);
    await expect(after.width).toBe(before.width);
  },
};
export const KeyboardShortcut: Story = {
  play: async ({ canvasElement }) => {
    if (window.innerWidth < 1012) {
      await expect(within(canvasElement).getByRole('button', { name: 'Open navigation' })).toBeVisible();
      return;
    }
    within(canvasElement).getByRole('main').focus();
    await userEvent.keyboard('{Control>}b{/Control}');
    await expect(within(canvasElement).getByTestId('console-sidebar')).toHaveAttribute('data-state', 'collapsed');
    await expect(localStorage.getItem('aib.sidebar-collapsed')).toBe('true');
  },
};
export const KeyboardSkip: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await expect(within(canvasElement).getByRole('main')).toHaveFocus();
  },
};
