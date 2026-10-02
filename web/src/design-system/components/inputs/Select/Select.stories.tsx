import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, waitFor, within } from 'storybook/test';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectGroup, SelectLabel, SelectItem } from './Select';

const meta = {
  title: 'Design System/Inputs/Select', component: Select, args: { defaultValue: 'any' }, parameters: { a11y: { test: 'error' } },
  render: (args) => <div className="w-64"><label htmlFor="match-mode" className="mb-2 block text-sm text-foreground">Match mode</label><Select {...args}><SelectTrigger id="match-mode"><SelectValue placeholder="Choose match mode" /></SelectTrigger><SelectContent><SelectGroup><SelectLabel>Matching</SelectLabel><SelectItem value="any">Any</SelectItem><SelectItem value="exact" disabled>Exact</SelectItem><SelectItem value="all">All</SelectItem></SelectGroup></SelectContent></Select></div>,
} satisfies Meta<typeof Select>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Hover: Story = { play: async ({ canvasElement }) => { await userEvent.hover(within(canvasElement).getByRole('combobox')); } };
export const Focus: Story = { play: async ({ canvasElement }) => { await userEvent.tab(); await expect(within(canvasElement).getByRole('combobox')).toHaveFocus(); } };
export const Disabled: Story = { args: { disabled: true } };
export const Loading: Story = { render: (args) => <Select {...args} disabled><SelectTrigger aria-label="Loading choices" aria-busy="true"><SelectValue placeholder="Loading choices" /></SelectTrigger><SelectContent><SelectItem value="any">Any</SelectItem></SelectContent></Select> };
export const Error: Story = { render: (args) => <div><Select {...args}><SelectTrigger aria-label="Match mode" aria-invalid="true" aria-describedby="match-error"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="any">Any</SelectItem></SelectContent></Select><p id="match-error" role="alert" className="text-sm text-destructive">Choose an available match mode</p></div> };
export const Open: Story = {
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await waitFor(async () => {
      await expect(body.getByRole('listbox')).toBeVisible();
      await expect(body.getByRole('option', { name: 'Any' })).toHaveFocus();
    });
  },
};

export const KeyboardSelection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.tab();
    await userEvent.keyboard('{Enter}');
    await waitFor(async () => {
      await expect(body.getByRole('listbox')).toBeVisible();
      await expect(body.getByRole('option', { name: 'Any' })).toHaveFocus();
    });
    await userEvent.keyboard('{ArrowDown}');
    await waitFor(async () => {
      await expect(body.getByRole('option', { name: 'All' })).toHaveFocus();
    });
    await userEvent.keyboard('{Enter}');
    await waitFor(async () => {
      await expect(canvas.getByRole('combobox')).toHaveTextContent('All');
      await expect(canvas.getByRole('combobox')).toHaveFocus();
      await expect(body.queryByRole('listbox')).not.toBeInTheDocument();
    });
    await userEvent.keyboard('{Enter}');
    await waitFor(async () => {
      await expect(body.getByRole('listbox')).toBeVisible();
      await expect(body.getByRole('option', { name: 'All' })).toHaveFocus();
    });
    await userEvent.keyboard('{Escape}');
    await waitFor(async () => {
      await expect(canvas.getByRole('combobox')).toHaveFocus();
      await expect(body.queryByRole('listbox')).not.toBeInTheDocument();
    });
  },
};
