import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import { TextArea } from './TextArea';

const meta = { title: 'Design System/Inputs/TextArea', component: TextArea, args: { label: 'Reason', description: 'Explain the requested access' }, parameters: { a11y: { test: 'error' } } } satisfies Meta<typeof TextArea>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Hover: Story = { play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('textbox')); } };
export const Focus: Story = { play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('textbox')).toHaveFocus(); } };
export const Editing: Story = { play: async ({ canvasElement }) => { const input = within(canvasElement).getByRole('textbox', { name: 'Reason' }); await userEvent.type(input, 'Read reports{Enter}Prepare a summary'); await expect(input).toHaveValue('Read reports\nPrepare a summary'); } };
export const Disabled: Story = { args: { disabled: true, defaultValue: 'Read only while saving' } };
export const Error: Story = { args: { error: 'Enter a reason' }, play: async ({ canvasElement }) => { await expect(within(canvasElement).getByRole('textbox')).toHaveAccessibleDescription('Enter a reason'); } };
export const Loading: Story = { args: { disabled: true, 'aria-busy': true, description: 'Saving reason' } };
