import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@design-system/theme/ThemeProvider';
import { DecisionShell } from './DecisionShell';

beforeEach(() => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  })));
});
afterEach(() => vi.unstubAllGlobals());

describe('DecisionShell', () => {
  it('groups local branding with the focused decision and no console navigation', () => {
    render(<ThemeProvider><DecisionShell wordmarkLabel="Agentic Identity Broker" skipToMainLabel="Skip to main content"><h1>Review access</h1><button>Allow</button></DecisionShell></ThemeProvider>);
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    const main = screen.getByRole('main');
    expect(main.parentElement).toContainElement(screen.getByRole('banner'));
    expect(screen.getByRole('banner')).toContainElement(screen.getByRole('img', { name: 'Agentic Identity Broker' }));
    expect(main).toContainElement(screen.getByRole('heading', { level: 1, name: 'Review access' }));
    expect(screen.queryByRole('contentinfo')).not.toBeInTheDocument();
  });

  it('keeps the shell focused on branding without account details in the chrome', () => {
    const { container } = render(<ThemeProvider><DecisionShell compact wordmarkLabel="Agentic Identity Broker" skipToMainLabel="Skip to main content"><h1>Review access</h1></DecisionShell></ThemeProvider>);
    expect(screen.getByRole('banner')).toContainElement(screen.getByRole('img', { name: 'Agentic Identity Broker' }));
    expect(screen.getByRole('banner')).not.toHaveTextContent('Signed in as');
    expect(container.querySelector('img[src^="http"]')).toBeNull();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('lets keyboard users skip branding and reach the decision without an invented footer', async () => {
    const user = userEvent.setup();
    render(<ThemeProvider><DecisionShell wordmarkLabel="Agentic Identity Broker" skipToMainLabel="Skip to main content"><h1>Review access</h1><button>Allow</button></DecisionShell></ThemeProvider>);
    await user.tab();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('main')).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Allow' })).toHaveFocus();
    expect(screen.queryByRole('contentinfo')).not.toBeInTheDocument();
  });
});
