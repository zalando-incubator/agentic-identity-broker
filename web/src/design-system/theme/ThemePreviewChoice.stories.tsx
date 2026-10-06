import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { ThemePreviewChoice, type ThemePreviewChoiceProps } from './ThemePreviewChoice';

const labels = { light: 'Light', dark: 'Dark', system: 'System' };

function InteractivePreview(props: ThemePreviewChoiceProps) {
  const [value, setValue] = useState(props.value);
  return <ThemePreviewChoice {...props} value={value} onValueChange={next => {
    setValue(next);
    props.onValueChange(next);
  }} />;
}

const meta = {
  title: 'Design System/Theme/ThemePreviewChoice',
  component: ThemePreviewChoice,
  parameters: { a11y: { test: 'error' } },
  args: { value: 'system', labels, 'aria-label': 'Appearance', onValueChange: fn() },
  render: (args) => <div className="w-full max-w-72"><InteractivePreview {...args} /></div>,
} satisfies Meta<typeof ThemePreviewChoice>;
export default meta;
type Story = StoryObj<typeof meta>;

export const System: Story = {};
export const Light: Story = { args: { value: 'light' }, globals: { theme: 'light' } };
export const Dark: Story = { args: { value: 'dark' }, globals: { theme: 'dark' } };
export const KeyboardSelection: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    canvas.getByRole('radio', { name: 'System' }).focus();
    await userEvent.keyboard('{ArrowLeft>}');
    try {
      await waitFor(() => expect(canvas.getByRole('radio', { name: 'Dark' })).toBeChecked());
    } finally {
      await userEvent.keyboard('{/ArrowLeft}');
    }
    await expect(args.onValueChange).toHaveBeenCalledWith('dark');
  },
};
