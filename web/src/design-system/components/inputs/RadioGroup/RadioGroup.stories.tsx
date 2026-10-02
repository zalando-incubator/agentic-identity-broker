import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { RadioGroup, RadioGroupItem } from './RadioGroup';

const meta = {
  title: 'Design System/Inputs/RadioGroup', component: RadioGroup, args: { defaultValue: 'system', 'aria-label': 'Appearance' }, parameters: { a11y: { test: 'error' } },
  render: (args) => <RadioGroup {...args}><label className="flex items-center gap-2 text-foreground"><RadioGroupItem value="system" />System</label><label className="flex items-center gap-2 text-foreground"><RadioGroupItem value="light" disabled />Light (unavailable)</label><label className="flex items-center gap-2 text-foreground"><RadioGroupItem value="dark" />Dark</label></RadioGroup>,
} satisfies Meta<typeof RadioGroup>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Hover: Story = { play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('radio', { name: 'System' })); } };
export const Focus: Story = { play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('radio', { name: 'System' })).toHaveFocus(); } };
export const Disabled: Story = { args: { disabled: true } };
export const Loading: Story = { args: { disabled: true, 'aria-busy': true } };
export const Error: Story = { args: { 'aria-invalid': true, 'aria-describedby': 'appearance-error' }, decorators: [(Story) => <><Story /><p id="appearance-error" role="alert" className="text-sm text-destructive">Choose an available appearance</p></>] };
export const KeyboardSelection: Story = { play: async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.tab();
  try {
    await userEvent.keyboard('{ArrowDown>}');
    await waitFor(async () => {
      await expect(canvas.getByRole('radio', { name: 'Dark' })).toHaveFocus();
      await expect(canvas.getByRole('radio', { name: 'Dark' })).toBeChecked();
    });
  } finally {
    await userEvent.keyboard('{/ArrowDown}');
  }
  await expect(canvas.getByRole('radio', { name: 'Dark' })).toHaveFocus();
  await expect(canvas.getByRole('radio', { name: 'Dark' })).toBeChecked();
  await expect(canvas.getByRole('radio', { name: 'System' })).not.toBeChecked();
} };
