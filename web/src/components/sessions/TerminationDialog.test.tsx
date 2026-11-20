import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { TerminationDialog } from './TerminationDialog';

const source = '<svg xmlns="http://www.w3.org/2000/svg"><circle r="3"/></svg>';
const replacement = '<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1h4v4z"/></svg>';

describe('TerminationDialog service identity', () => {
  it('decorates only the identity header and preserves prose, name-only fallback and confirmation controls', () => {
    const onConfirm = vi.fn();
    const onClose = vi.fn();
    const view = (serviceIconSvg?: string | null) => (
      <TerminationDialog isOpen onConfirm={onConfirm} onClose={onClose}
        serviceName="Atlas Files" serviceIconSvg={serviceIconSvg}
        dependentAgents={[]} loading={false} />
    );
    const { rerender } = render(view(source));
    const dialog = screen.getByRole('dialog');
    const heading = within(dialog).getByRole('heading', { name: 'Atlas Files' });
    const confirm = within(dialog).getByRole('button', { name: 'Terminate session with Atlas Files' });
    const cancel = within(dialog).getByRole('button', { name: 'Cancel session termination' });
    const prose = dialog.querySelector('#termination-dialog-description')!;
    expect(prose).toHaveTextContent('Are you sure you want to terminate your session with Atlas Files?');
    expect(prose.querySelector('img')).toBeNull();
    const icon = dialog.querySelector('img')!;
    expect(icon).toHaveAttribute('alt', '');
    expect(heading.parentElement).toContainElement(icon);
    fireEvent.error(icon);
    expect(dialog.querySelector('img')).toBeNull();
    expect(heading).toBeVisible();
    rerender(view(replacement));
    const next = dialog.querySelector('img')!;
    expect(next).toHaveAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(replacement)}`);
    fireEvent.load(next);
    expect(next).toBeVisible();
    rerender(view(null));
    expect(dialog.querySelector('img')).toBeNull();
    expect(within(dialog).getByRole('button', { name: 'Terminate session with Atlas Files' })).toBe(confirm);
    expect(within(dialog).getByRole('button', { name: 'Cancel session termination' })).toBe(cancel);
    fireEvent.click(confirm);
    fireEvent.click(cancel);
    expect(onConfirm).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
    expect(screen.queryByRole('alert', { name: /failed/i })).toBeNull();
  });
});
