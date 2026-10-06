import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { ConsoleShell, type ConsoleShellProps } from './ConsoleShell';

const labels: ConsoleShellProps['labels'] = {
  wordmark: 'Agentic Identity Broker', skipToMain: 'Skip to main content', navigation: 'Main navigation',
  collapseSidebar: 'Collapse sidebar', expandSidebar: 'Expand sidebar', openNavigation: 'Open navigation',
  closeNavigation: 'Close navigation', navigationDescription: 'Choose a console page.',
  pendingCount: (count) => `${count} pending approvals`, pendingUnknown: 'Pending approvals unavailable',
  pendingStale: 'Last known count; refresh unavailable',
};
const props: ConsoleShellProps = {
  labels,
  navigation: [
    { href: '/agents', label: 'Agents', icon: <span>A</span> },
    { href: '/connections', label: 'Connections', icon: <span>C</span> },
    { href: '/approvals', label: 'Approvals', icon: <span>P</span>, pendingApprovals: true },
  ],
  search: <button>Search records</button>, userMenu: <button>User menu</button>,
  pendingCount: 3, children: <button>Page action</button>,
};
function Route() { return <output aria-label="Current route">{useLocation().pathname}</output>; }
function Shell(overrides: Partial<ConsoleShellProps> = {}) {
  return <MemoryRouter initialEntries={['/agents']}><ThemeProvider><ConsoleShell {...props} {...overrides} /><Route /></ThemeProvider></MemoryRouter>;
}
function viewport(narrow: boolean) {
  vi.stubGlobal('matchMedia', vi.fn((query: string) => ({
    matches: query === '(width < 63.25rem)' && narrow, media: query,
    addEventListener: vi.fn(), removeEventListener: vi.fn(),
  })));
}
beforeEach(() => { localStorage.clear(); viewport(false); });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe('ConsoleShell', () => {
  it('skips navigation by keyboard and continues tabbing in the main content', async () => {
    const user = userEvent.setup();
    render(Shell());
    await user.tab();
    expect(screen.getByRole('link', { name: labels.skipToMain })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('main')).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Page action' })).toHaveFocus();
  });

  it('navigates in the client router with a stable link name and descriptive pending count', async () => {
    const user = userEvent.setup();
    render(Shell());
    const nav = screen.getByRole('navigation', { name: labels.navigation });
    const approvals = within(nav).getByRole('link', { name: 'Approvals', exact: true });
    expect(approvals).toHaveAccessibleDescription('3 pending approvals');
    expect(screen.getByTestId('pending-approval-count')).toHaveTextContent(/^3$/);
    expect(within(nav).getByRole('link', { name: 'Agents' })).toHaveAttribute('aria-current', 'page');
    await user.click(approvals);
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/approvals');
    expect(approvals).toHaveAttribute('aria-current', 'page');
  });

  it('persists collapse across remounts while retaining accessible navigation and compact branding', async () => {
    const user = userEvent.setup();
    const first = render(Shell());
    await user.click(screen.getByRole('button', { name: labels.collapseSidebar }));
    expect(localStorage.getItem('aib.sidebar-collapsed')).toBe('true');
    first.unmount();
    render(Shell());
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'collapsed');
    expect(screen.getByRole('img', { name: labels.wordmark })).toHaveAttribute('data-variant', 'compact');
    expect(screen.getByRole('link', { name: 'Connections' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: labels.expandSidebar }));
    expect(localStorage.getItem('aib.sidebar-collapsed')).toBe('false');
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'expanded');
  });

  it('toggles and persists collapse with Cmd/Ctrl+B except in editable targets', async () => {
    const user = userEvent.setup();
    render(Shell({ search: <input aria-label="Search records" />, children: <div contentEditable role="textbox" aria-label="Editor"><span>Editable text</span></div> }));
    await user.keyboard('{Control>}b{/Control}');
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'collapsed');
    expect(localStorage.getItem('aib.sidebar-collapsed')).toBe('true');
    await user.keyboard('{Meta>}b{/Meta}');
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'expanded');
    await user.click(screen.getByRole('textbox', { name: 'Search records' }));
    await user.keyboard('{Control>}b{/Control}');
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'expanded');
    await user.click(screen.getByText('Editable text'));
    await user.keyboard('{Meta>}b{/Meta}');
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'expanded');
    expect(localStorage.getItem('aib.sidebar-collapsed')).toBe('false');
  });

  it('does not hide mobile navigation with the desktop collapse shortcut', async () => {
    viewport(true);
    const user = userEvent.setup();
    render(Shell());
    await user.keyboard('{Control>}b{/Control}');
    expect(localStorage.getItem('aib.sidebar-collapsed')).toBeNull();
    expect(screen.getByRole('button', { name: labels.openNavigation })).toBeInTheDocument();
  });

  it('rejects malformed storage and keeps collapse usable if browser storage is blocked', async () => {
    const user = userEvent.setup();
    localStorage.setItem('aib.sidebar-collapsed', '"true"');
    const first = render(Shell());
    expect(screen.getByRole('button', { name: labels.collapseSidebar })).toBeInTheDocument();
    first.unmount();
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Blocked', 'SecurityError'); });
    render(Shell());
    await user.click(screen.getByRole('button', { name: labels.collapseSidebar }));
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'collapsed');
    await user.click(screen.getByRole('button', { name: labels.expandSidebar }));
    expect(screen.getByTestId('console-sidebar')).toHaveAttribute('data-state', 'expanded');
  });

  it('does not turn an unavailable count into zero and announces updates without taking focus', () => {
    const view = render(Shell({ pendingCount: undefined }));
    expect(screen.queryByTestId('pending-approval-count')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Approvals' })).toHaveAccessibleDescription(labels.pendingUnknown);
    const search = screen.getByRole('button', { name: 'Search records' });
    act(() => search.focus());
    view.rerender(Shell({ pendingCount: 4, pendingStale: true, announcement: 'A new request needs your review.' }));
    expect(screen.getByRole('button', { name: labels.pendingStale })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Approvals' })).toHaveAccessibleDescription(`4 pending approvals ${labels.pendingStale}`);
    expect(screen.getByTestId('approval-announcement')).toHaveAttribute('aria-live', 'polite');
    expect(screen.getByTestId('approval-announcement')).toHaveTextContent('A new request needs your review.');
    expect(search).toHaveFocus();
    expect(screen.getByRole('button', { name: 'User menu' })).toBeInTheDocument();
  });

  it('hides the visual pending count at zero while exposing stale details on the warning icon', async () => {
    const user = userEvent.setup();
    render(Shell({ pendingCount: 0, pendingStale: true }));
    expect(screen.queryByTestId('pending-approval-count')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Approvals' })).toHaveAccessibleDescription(`0 pending approvals ${labels.pendingStale}`);
    await user.hover(screen.getByRole('button', { name: labels.pendingStale }));
    expect(await screen.findByRole('tooltip')).toHaveTextContent(labels.pendingStale);
  });

  it('opens mobile navigation by keyboard, traps focus, and returns focus after Escape', async () => {
    viewport(true);
    const user = userEvent.setup();
    render(Shell());
    expect(window.matchMedia).toHaveBeenCalledWith('(width < 63.25rem)');
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    const opener = screen.getByRole('button', { name: labels.openNavigation });
    act(() => opener.focus());
    await user.keyboard('{Enter}');
    const dialog = screen.getByRole('dialog', { name: labels.navigation });
    expect(dialog).toHaveAccessibleDescription(labels.navigationDescription);
    expect(within(dialog).getByRole('button', { name: 'Search records' })).toHaveFocus();
    await user.tab({ shift: true });
    expect(within(dialog).getByRole('button', { name: labels.closeNavigation })).toHaveFocus();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(opener).toHaveFocus();
  });

  it('closes the mobile Sheet after client navigation and restores its trigger', async () => {
    viewport(true);
    const user = userEvent.setup();
    render(Shell());
    const opener = screen.getByRole('button', { name: labels.openNavigation });
    await user.click(opener);
    await user.click(screen.getByRole('link', { name: 'Connections' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/connections');
  });
});
