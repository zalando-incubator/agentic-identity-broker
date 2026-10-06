import { useState } from 'react';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemePreviewChoice } from './ThemePreviewChoice';
import type { ThemePreference } from './themePreference';

const labels = { light: 'Clair', dark: 'Sombre', system: 'Système' };

afterEach(cleanup);

function ControlledPreview({ onChange }: { onChange: (value: ThemePreference) => void }) {
  const [value, setValue] = useState<ThemePreference>('system');
  return <ThemePreviewChoice value={value} labels={labels} aria-label="Apparence" onValueChange={next => {
    setValue(next);
    onChange(next);
  }} />;
}

describe('ThemePreviewChoice', () => {
  it('shows all three labeled choices and a distinct light, dark, or split preview', () => {
    render(<ThemePreviewChoice value="system" labels={labels} aria-label="Apparence" onValueChange={vi.fn()} />);
    const group = screen.getByRole('radiogroup', { name: 'Apparence' });
    const light = within(group).getByRole('radio', { name: labels.light });
    const dark = within(group).getByRole('radio', { name: labels.dark });
    const system = within(group).getByRole('radio', { name: labels.system });
    expect(system).toBeChecked();
    expect(light).not.toBeChecked();
    expect(dark).not.toBeChecked();
    expect(Array.from(light.querySelectorAll('[data-theme]'), tile => tile.getAttribute('data-theme'))).toEqual(['light', 'light']);
    expect(Array.from(dark.querySelectorAll('[data-theme]'), tile => tile.getAttribute('data-theme'))).toEqual(['dark', 'dark']);
    expect(Array.from(system.querySelectorAll('[data-theme]'), tile => tile.getAttribute('data-theme'))).toEqual(['light', 'dark']);
  });

  it('reports the selected preference, but leaves selection controlled by the caller', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const view = render(<ThemePreviewChoice value="light" labels={labels} aria-label="Apparence" onValueChange={onChange} />);
    await user.click(screen.getByRole('radio', { name: labels.dark }));
    expect(onChange).toHaveBeenCalledWith('dark');
    expect(screen.getByRole('radio', { name: labels.light })).toBeChecked();
    view.rerender(<ThemePreviewChoice value="dark" labels={labels} aria-label="Apparence" onValueChange={onChange} />);
    expect(screen.getByRole('radio', { name: labels.dark })).toBeChecked();
  });

  it('selects tiles using arrow keys without submitting an enclosing form', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const submit = vi.fn((event: React.FormEvent) => event.preventDefault());
    render(<form onSubmit={submit}><ControlledPreview onChange={onChange} /></form>);
    screen.getByRole('radio', { name: labels.system }).focus();
    try {
      await user.keyboard('{ArrowLeft>}');
      await waitFor(() => expect(screen.getByRole('radio', { name: labels.dark })).toBeChecked());
    } finally {
      await user.keyboard('{/ArrowLeft}');
    }
    expect(screen.getByRole('radio', { name: labels.dark })).toHaveFocus();
    expect(onChange).toHaveBeenCalledWith('dark');
    expect(submit).not.toHaveBeenCalled();
  });
});
