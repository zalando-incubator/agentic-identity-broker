import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { TruncatedText } from './TruncatedText';

const labels = { expandLabel: 'Show full name', collapseLabel: 'Show less' };

describe('TruncatedText', () => {
  it.each([1, 2] as const)('expands and collapses %s-line metadata by keyboard without submitting a form', async (lines) => {
    const user = userEvent.setup();
    let submissions = 0;
    render(<form onSubmit={(event) => { event.preventDefault(); submissions += 1; }}>
      <TruncatedText text="An exceptionally detailed service identity" lines={lines} {...labels} />
    </form>);
    const toggle = screen.getByRole('button', { name: labels.expandLabel });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(document.getElementById(toggle.getAttribute('aria-controls')!)).toHaveTextContent('An exceptionally detailed service identity');
    await user.tab();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('button', { name: labels.collapseLabel })).toHaveAttribute('aria-expanded', 'true');
    expect(toggle).toHaveFocus();
    await user.keyboard(' ');
    expect(screen.getByRole('button', { name: labels.expandLabel })).toHaveAttribute('aria-expanded', 'false');
    expect(submissions).toBe(0);
  });

  it('keeps the exact identity heading separate from its expansion action', async () => {
    const user = userEvent.setup();
    render(<TruncatedText as="h1" text="Research assistant" {...labels} />);
    const heading = screen.getByRole('heading', { level: 1, name: 'Research assistant' });
    const toggle = screen.getByRole('button', { name: labels.expandLabel });
    expect(heading).toHaveTextContent(/^Research assistant$/);
    expect(heading).not.toContainElement(toggle);
    expect(toggle).toHaveAttribute('aria-controls', heading.id);
    await user.click(toggle);
    expect(screen.getByRole('heading', { level: 1 })).toHaveAccessibleName('Research assistant');
    expect(heading).toHaveTextContent(/^Research assistant$/);
  });

  it('renders untrusted markup only as text and gives each instance its own controlled content', async () => {
    const user = userEvent.setup();
    const attack = '<img src="https://untrusted.invalid/pixel" onerror="alert(1)"><script>alert(2)</script>';
    const { container } = render(<>
      <TruncatedText text={attack} {...labels} />
      <TruncatedText text="A separate identity" expandLabel="Expand second" collapseLabel="Collapse second" />
    </>);
    await user.click(screen.getByRole('button', { name: labels.expandLabel }));
    expect(screen.getByText(attack)).toBeVisible();
    expect(container.querySelector('img, script')).toBeNull();
    const first = screen.getByRole('button', { name: labels.collapseLabel });
    const second = screen.getByRole('button', { name: 'Expand second' });
    expect(first.getAttribute('aria-controls')).not.toBe(second.getAttribute('aria-controls'));
    expect(second).toHaveAttribute('aria-expanded', 'false');
  });
});
