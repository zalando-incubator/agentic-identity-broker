import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { Input } from './Input';

const meta = { title: 'Design System/Inputs/Input', component: Input, args: { label: 'Display name', description: 'Visible to your team' }, parameters: { a11y: { test: 'error' } } } satisfies Meta<typeof Input>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Hover: Story = { play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('textbox')); } };
export const Focus: Story = { play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('textbox')).toHaveFocus(); } };
export const Editing: Story = { play: async ({ canvasElement }) => { const input = within(canvasElement).getByRole('textbox', { name: 'Display name' }); await userEvent.type(input, 'Ada'); await expect(input).toHaveValue('Ada'); } };
export const Disabled: Story = { args: { disabled: true, defaultValue: 'Read only while saving' } };
export const Error: Story = { args: { error: 'Enter your display name' }, play: async ({ canvasElement }) => { await expect(within(canvasElement).getByRole('textbox')).toHaveAccessibleDescription('Enter your display name'); } };
export const Loading: Story = { args: { disabled: true, 'aria-busy': true, description: 'Saving display name' } };
