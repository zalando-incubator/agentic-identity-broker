import { useState } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Input } from './Input';
import { TextArea } from '../TextArea/TextArea';
import { Checkbox } from '../Checkbox/Checkbox';
import { Switch } from '../Switch/Switch';
import { RadioGroup, RadioGroupItem } from '../RadioGroup/RadioGroup';
import { DatePicker } from '../DatePicker/DatePicker';

describe('owned input behavior', () => {
  it.each([Input, TextArea])('associates its label and error while preserving external descriptions', async (Control) => {
    const user = userEvent.setup();
    const { rerender } = render(<><p id="policy">Not shared publicly</p><Control label="Display name" error="Enter a name" aria-describedby="policy" /></>);
    const input = screen.getByRole('textbox', { name: 'Display name' });
    expect(input).toHaveAccessibleDescription('Not shared publicly Enter a name');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    await user.click(screen.getByText('Display name'));
    expect(input).toHaveFocus();
    await user.type(input, 'Ada');
    expect(input).toHaveValue('Ada');
    rerender(<><p id="policy">Not shared publicly</p><Control label="Display name" description="Your public name" aria-describedby="policy" /></>);
    expect(screen.getByRole('textbox', { name: 'Display name' })).toHaveAccessibleDescription('Not shared publicly Your public name');
    expect(screen.getByRole('textbox')).not.toHaveAttribute('aria-invalid', 'true');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it.each([[Checkbox, 'checkbox'], [Switch, 'switch']] as const)('changes the controlled state with Space and blocks disabled activation', async (Control, role) => {
    const user = userEvent.setup();
    const changed = vi.fn();
    function Example({ disabled = false }: { disabled?: boolean }) {
      const [checked, setChecked] = useState(false);
      return <Control aria-label="Remember choice" checked={checked} disabled={disabled} onCheckedChange={(next) => { changed(next); setChecked(next === true); }} />;
    }
    const { rerender } = render(<Example />);
    await user.tab();
    await user.keyboard(' ');
    expect(screen.getByRole(role, { name: 'Remember choice' })).toBeChecked();
    expect(changed).toHaveBeenCalledWith(true);
    rerender(<Example disabled />);
    await user.click(screen.getByRole(role));
    expect(changed).toHaveBeenCalledTimes(1);
    expect(screen.getByRole(role)).toBeChecked();
  });

  it('announces mixed checkbox state and resolves it through its visible label', async () => {
    const user = userEvent.setup();
    function Example() {
      const [checked, setChecked] = useState<boolean | 'indeterminate'>('indeterminate');
      return <Checkbox label="Optional permissions" checked={checked} onCheckedChange={setChecked} error={checked === 'indeterminate' ? 'Review your selections' : undefined} />;
    }
    render(<Example />);
    const checkbox = screen.getByRole('checkbox', { name: 'Optional permissions' });
    expect(checkbox).toHaveAttribute('aria-checked', 'mixed');
    expect(checkbox).toHaveAccessibleDescription('Review your selections');
    await user.click(screen.getByText('Optional permissions'));
    expect(checkbox).toBeChecked();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('moves radio selection with arrows and skips unavailable choices', async () => {
    const user = userEvent.setup();
    function Example() {
      const [value, setValue] = useState('system');
      return <RadioGroup aria-label="Appearance" value={value} onValueChange={setValue}><RadioGroupItem value="system" aria-label="System" /><RadioGroupItem value="light" aria-label="Light" disabled /><RadioGroupItem value="dark" aria-label="Dark" /></RadioGroup>;
    }
    render(<Example />);
    await user.tab();
    expect(screen.getByRole('radio', { name: 'System' })).toHaveFocus();
    try {
      await user.keyboard('{ArrowDown>}');
      await waitFor(() => {
        expect(screen.getByRole('radio', { name: 'Dark' })).toHaveFocus();
        expect(screen.getByRole('radio', { name: 'Dark' })).toBeChecked();
      });
    } finally {
      await user.keyboard('{/ArrowDown}');
    }
    expect(screen.getByRole('radio', { name: 'Dark' })).toHaveFocus();
    expect(screen.getByRole('radio', { name: 'Dark' })).toBeChecked();
    expect(screen.getByRole('radio', { name: 'System' })).not.toBeChecked();
  });

  it('keeps an invalid date visible without committing it, then accepts the inclusive minimum', () => {
    const changed = vi.fn();
    render(<DatePicker label="Expiration" value="" onValueChange={changed} min="2026-10-02" validationMessage="Choose October 2 or later" />);
    const input = screen.getByLabelText('Expiration');
    fireEvent.change(input, { target: { value: '2026-10-01' } });
    expect(changed).not.toHaveBeenCalled();
    expect(input).toHaveValue('2026-10-01');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAccessibleDescription('Choose October 2 or later');
    expect(screen.getByRole('alert')).toHaveTextContent('Choose October 2 or later');
    fireEvent.change(input, { target: { value: '2026-10-02' } });
    expect(changed).toHaveBeenLastCalledWith('2026-10-02');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('rejects dates after the maximum and distinguishes required from optional clearing', () => {
    const changed = vi.fn();
    const { rerender } = render(<DatePicker label="Expiration" value="2026-10-02" onValueChange={changed} max="2026-10-31" required validationMessage="Choose a date in October" />);
    fireEvent.change(screen.getByLabelText('Expiration'), { target: { value: '2026-11-01' } });
    expect(changed).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('Expiration'), { target: { value: '' } });
    expect(changed).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Expiration')).toHaveAccessibleDescription('Choose a date in October');
    rerender(<DatePicker label="Expiration" value="2026-10-03" onValueChange={changed} validationMessage="Choose a date" />);
    expect(screen.getByLabelText('Expiration')).toHaveValue('2026-10-03');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Expiration'), { target: { value: '' } });
    expect(changed).toHaveBeenLastCalledWith('');
  });
});
