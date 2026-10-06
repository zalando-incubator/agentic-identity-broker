import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Checkbox } from './Checkbox';

const meta = { title: 'Design System/Inputs/Checkbox', component: Checkbox, args: { label: 'Remember choice', description: 'Only on this browser' }, parameters: { a11y: { test: 'error' } } } satisfies Meta<typeof Checkbox>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Checked: Story = { args: { defaultChecked: true } };
export const Hover: Story = { play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('checkbox')); } };
export const Focus: Story = { play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('checkbox')).toHaveFocus(); } };
export const Disabled: Story = { args: { disabled: true, defaultChecked: true } };
export const Error: Story = { args: { error: 'Could not save your choice' }, play: async ({ canvasElement }) => { await expect(within(canvasElement).getByRole('checkbox')).toHaveAccessibleDescription('Could not save your choice'); } };
export const Loading: Story = { args: { disabled: true, 'aria-busy': true, description: 'Saving choice' } };
export const KeyboardToggle: Story = { play: async ({ canvasElement }) => { const control = within(canvasElement).getByRole('checkbox'); await userEvent.tab(); await userEvent.keyboard(' '); await expect(control).toBeChecked(); await userEvent.keyboard(' '); await expect(control).not.toBeChecked(); } };
export const Indeterminate: Story = { args: { checked: 'indeterminate' } };
