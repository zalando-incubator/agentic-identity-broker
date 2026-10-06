import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { TruncatedText } from './TruncatedText';

const labels = { expandLabel: 'More', collapseLabel: 'Less' };
afterEach(() => vi.restoreAllMocks());

function measure(overflow: boolean) {
  let clipped = overflow;
  const callbacks = new Set<ResizeObserverCallback>();
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(40);
  vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockImplementation(() => clipped ? 80 : 40);
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(200);
  vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(() => clipped ? 400 : 200);
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback: ResizeObserverCallback) { callbacks.add(callback); }
    observe() {}
    disconnect() {}
  });
  return (next: boolean) => {
    clipped = next;
    act(() => callbacks.forEach(callback => callback([], {} as ResizeObserver)));
  };
}

afterEach(() => vi.unstubAllGlobals());

describe('measured truncation', () => {
  it('shows no expansion for text that fits and responds when its width changes', () => {
    const resize = measure(false);
    render(<TruncatedText text="Description" {...labels} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    resize(true);
    expect(screen.getByRole('button', { name: 'More' })).toHaveAttribute('aria-expanded', 'false');
    resize(false);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    resize(true);
    expect(screen.getByRole('button', { name: 'More' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('expands clipped descriptions by keyboard without submitting a surrounding form', async () => {
    measure(true);
    const user = userEvent.setup();
    const submit = vi.fn(event => event.preventDefault());
    render(<form onSubmit={submit}><TruncatedText text="A long description" lines={2} {...labels} /></form>);
    const toggle = screen.getByRole('button', { name: 'More' });
    const contentId = toggle.getAttribute('aria-controls');
    expect(contentId).toBeTruthy();
    expect(document.getElementById(contentId!)).toHaveTextContent('A long description');
    await user.tab();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('button', { name: 'Less' })).toHaveAttribute('aria-expanded', 'true');
    expect(toggle).toHaveFocus();
    await user.keyboard(' ');
    expect(screen.getByRole('button', { name: 'More' })).toHaveAttribute('aria-expanded', 'false');
    expect(submit).not.toHaveBeenCalled();
  });

  it('offers clipped single-line names in a tooltip without an expansion control', async () => {
    measure(true);
    const user = userEvent.setup();
    render(<TruncatedText text="A detailed agent name" lines={1} {...labels} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    await user.tab();
    expect(await screen.findByRole('tooltip')).toHaveTextContent('A detailed agent name');
  });

  it('reports clipping without adding nested controls inside clickable names', () => {
    const resize = measure(true);
    const onClippedChange = vi.fn();
    render(<TruncatedText text="An embedded name" lines={2} interactive={false} onClippedChange={onClippedChange} />);
    expect(screen.getByText('An embedded name')).not.toHaveAttribute('tabindex');
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(onClippedChange).toHaveBeenCalledWith(true);
    resize(false);
    expect(onClippedChange).toHaveBeenLastCalledWith(false);
  });

  it('escapes untrusted text without making a fitting heading interactive', () => {
    measure(false);
    const attack = '<img src="https://untrusted.invalid/pixel"><script>alert(1)</script>';
    const { container } = render(<TruncatedText as="h1" text={attack} {...labels} />);
    expect(screen.getByRole('heading', { name: attack })).toBeVisible();
    expect(container.querySelector('img, script')).toBeNull();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
