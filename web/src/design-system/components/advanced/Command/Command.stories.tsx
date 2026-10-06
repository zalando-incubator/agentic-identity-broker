import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, userEvent, within } from 'storybook/test';
import {
  Command, CommandEmpty, CommandGroup, CommandInput, CommandItem,
  CommandList, CommandLoading, CommandSeparator, CommandShortcut,
} from './Command';

function SearchExample() {
  const [selected, setSelected] = useState('');
  return (
    <div className="w-full max-w-md space-y-3">
      <Command label="Search console" loop>
        <CommandInput placeholder="Search agents or appearance…" />
        <CommandList label="Console results">
          <CommandGroup heading="Agents">
            <CommandItem value="calendar" keywords={['meetings']} onSelect={setSelected}>Calendar assistant</CommandItem>
            <CommandItem value="mail" onSelect={setSelected}>Mail assistant</CommandItem>
            <CommandItem value="unavailable" disabled onSelect={setSelected}>Unavailable assistant</CommandItem>
          </CommandGroup>
          <CommandSeparator />
          <CommandGroup heading="Appearance">
            <CommandItem value="dark" onSelect={setSelected}>Dark appearance<CommandShortcut>⌘D</CommandShortcut></CommandItem>
          </CommandGroup>
        </CommandList>
        <CommandEmpty>No matching records.</CommandEmpty>
      </Command>
      {selected && <p role="status" aria-label="Selection" className="text-sm text-muted-foreground">Selected: {selected}</p>}
    </div>
  );
}

const meta = {
  title: 'Advanced/Command',
  component: Command,
  tags: ['autodocs'],
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  args: { label: 'Search console' },
  render: () => <SearchExample />,
} satisfies Meta<typeof Command>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    const option = within(canvasElement).getByRole('option', { name: 'Mail assistant' });
    await userEvent.hover(option);
    await expect(option).toHaveAttribute('aria-selected', 'true');
  },
};
export const Focus: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('combobox', { name: 'Search console' })).toHaveFocus();
  },
};
export const KeyboardSelection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByRole('combobox', { name: 'Search console' });
    await userEvent.click(input);
    await userEvent.keyboard('{ArrowDown}{Enter}');
    await expect(canvas.getByRole('status', { name: 'Selection' })).toHaveTextContent('Selected: mail');
    await expect(input).toHaveFocus();
  },
};
export const Disabled: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const option = canvas.getByRole('option', { name: 'Unavailable assistant' });
    await expect(option).toHaveAttribute('aria-disabled', 'true');
    await userEvent.hover(option);
    await expect(option).toHaveAttribute('aria-selected', 'false');
  },
};
export const Empty: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByRole('combobox', { name: 'Search console' }), 'zzzzzz');
    await expect(canvas.getByText('No matching records.')).toBeVisible();
    await expect(canvas.queryByRole('option')).not.toBeInTheDocument();
    await expect(canvas.getByRole('combobox', { name: 'Search console' })).toHaveAttribute('aria-expanded', 'false');
    await expect(canvas.queryByRole('listbox')).not.toBeInTheDocument();
  },
};
export const Loading: Story = {
  render: () => (
    <Command label="Search console" className="max-w-md">
      <CommandInput placeholder="Search agents…" />
      <CommandList label="Console results" aria-busy="true" />
      <CommandLoading label="Loading agents">Loading agents…</CommandLoading>
    </Command>
  ),
};
export const Error: Story = {
  render: () => (
    <div className="w-full max-w-md space-y-2">
      <Command label="Search console">
        <CommandInput aria-invalid="true" aria-describedby="command-search-error" />
        <CommandList label="Console results" />
      </Command>
      <p id="command-search-error" role="alert" className="text-sm text-status-danger-foreground">Search results could not be loaded.</p>
    </div>
  ),
};
