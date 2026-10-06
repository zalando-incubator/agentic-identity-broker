import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fireEvent, fn, userEvent, waitFor, within } from 'storybook/test';
import { Button } from '@design-system/components/primitives/Button/Button';
import { DatePicker } from './DatePicker';

const meta = {
  title: 'Design System/Inputs/DatePicker', component: DatePicker,
  args: { label: 'Expiration', value: '', min: '2026-10-02', max: '2026-10-31', validationMessage: 'Choose a date from October 2 through October 31', onValueChange: () => {} },
  parameters: { a11y: { test: 'error' } },
  render: function Example(args) { const [value, setValue] = useState(args.value); return <DatePicker {...args} value={value} onValueChange={setValue} />; },
} satisfies Meta<typeof DatePicker>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Hover: Story = { play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByLabelText('Expiration')); } };
export const Focus: Story = { play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByLabelText('Expiration')).toHaveFocus(); } };
export const Disabled: Story = { args: { disabled: true, value: '2026-10-15' } };
export const Loading: Story = { args: { disabled: true, 'aria-busy': true, description: 'Saving expiration' } };
export const Error: Story = { args: { error: 'Expiration could not be saved' } };
export const DateBoundaries: Story = { play: async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  const input = canvas.getByLabelText('Expiration');
  fireEvent.change(input, { target: { value: '2026-10-01' } });
  await expect(input).toHaveAttribute('aria-invalid', 'true');
  await expect(input).toHaveAccessibleDescription('Choose a date from October 2 through October 31');
  fireEvent.change(input, { target: { value: '2026-10-02' } });
  await expect(input).toHaveValue('2026-10-02');
  await expect(canvas.queryByRole('alert')).not.toBeInTheDocument();
} };

export const NativeDatePartsFocus: Story = {
  args: { value: '2026-10-15', onValueChange: fn() },
  render: (args) => (
    <div className="space-y-4">
      <Button variant="outline">Before expiration</Button>
      <DatePicker {...args} />
      <Button variant="outline">After expiration</Button>
    </div>
  ),
  play: async ({ canvasElement, args }) => {
    // vitest/browser cannot be imported statically outside its native test runner.
    // Vite removes this branch/import from standalone Storybook; both browser
    // test projects use test mode and exercise the real native date subcontrols.
    if (import.meta.env.MODE !== 'test') return;
    const { userEvent: nativeUserEvent } = await import('vitest/browser');
    const canvas = within(canvasElement);
    const input = canvas.getByLabelText('Expiration');
    const after = canvas.getByRole('button', { name: 'After expiration' });
    // Resolve the token as a color so browser serialization (0.10 versus 0.1)
    // does not turn the dark-theme ring assertion into a formatting check.
    const colorProbe = canvasElement.ownerDocument.createElement('span');
    colorProbe.hidden = true;
    colorProbe.style.color = 'var(--ring)';
    canvasElement.append(colorProbe);
    const ringColor = getComputedStyle(colorProbe).color;
    colorProbe.remove();
    const expectDateFocusRing = async () => {
      await expect(input).toHaveFocus();
      const style = getComputedStyle(input);
      const ringShadow = style.boxShadow
        .split(/,(?![^()]*\))/)
        .find((shadow) => shadow.trim().startsWith(ringColor));
      // Require a visible token-colored ring, not just a matching focus pseudo.
      await expect(ringShadow).toMatch(/ 0px 0px 0px [1-9][\d.]*px$/);
      await expect(input).toHaveValue('2026-10-15');
      await expect(args.onValueChange).not.toHaveBeenCalled();
    };

    await nativeUserEvent.click(canvas.getByRole('button', { name: 'Before expiration' }));
    // Chromium/en-US exposes month, day, year, then the native calendar button.
    for (let part = 0; part < 4; part += 1) {
      await nativeUserEvent.tab();
      await waitFor(expectDateFocusRing);
    }
    await nativeUserEvent.tab();
    await expect(after).toHaveFocus();
    await expect(getComputedStyle(input).boxShadow).toBe('none');
    // Leave the calendar button focused for the light/dark visual baseline.
    await nativeUserEvent.tab({ shift: true });
    await waitFor(expectDateFocusRing);
  },
};
