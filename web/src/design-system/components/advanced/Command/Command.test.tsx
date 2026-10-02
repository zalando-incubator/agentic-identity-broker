import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandLoading, CommandSeparator } from './Command';


function SearchCommands({ onSelect }: { onSelect: (value: string) => void }) {
  return (
    <Command label="Search console" loop>
      <CommandInput />
      <CommandList label="Console results">
        <CommandGroup heading="Agents">
          <CommandItem value="calendar" keywords={['meetings']} onSelect={onSelect}>Calendar assistant</CommandItem>
          <CommandItem value="unavailable" disabled onSelect={onSelect}>Unavailable assistant</CommandItem>
          <CommandItem value="mail" onSelect={onSelect}>Mail assistant</CommandItem>
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Appearance">
          <CommandItem value="dark" onSelect={onSelect}>Dark appearance</CommandItem>
        </CommandGroup>
      </CommandList>
      <CommandEmpty>No matching records</CommandEmpty>
    </Command>
  );
}

describe('Command', () => {
  it('filters by keywords, hides unrelated groups, and recovers from no results', async () => {
    const user = userEvent.setup();
    render(<SearchCommands onSelect={vi.fn()} />);
    const input = screen.getByRole('combobox', { name: 'Search console' });
    await user.type(input, 'meetings');
    expect(screen.getByRole('option', { name: 'Calendar assistant' })).toBeVisible();
    expect(screen.queryByRole('option', { name: 'Mail assistant' })).not.toBeInTheDocument();
    expect(screen.queryByRole('group', { name: 'Appearance' })).not.toBeInTheDocument();
    await user.clear(input);
    await user.type(input, 'zzzzzz');
    expect(screen.getByRole('status')).toHaveTextContent('No matching records');
    expect(input).toHaveAttribute('aria-expanded', 'false');
    expect(input).not.toHaveAttribute('aria-activedescendant');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('option')).not.toBeInTheDocument();
    await user.clear(input);
    expect(screen.getByRole('option', { name: 'Mail assistant' })).toBeVisible();
    expect(input).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('listbox', { name: 'Console results' })).toBeVisible();
    expect(screen.queryByText('No matching records')).not.toBeInTheDocument();
  });

  it('keeps combobox focus, skips disabled options, and selects the active result with Enter', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<SearchCommands onSelect={onSelect} />);
    const input = screen.getByRole('combobox', { name: 'Search console' });
    expect(screen.queryByRole('separator')).not.toBeInTheDocument();
    await user.click(input);
    await waitFor(() => expect(screen.getByRole('option', { name: 'Calendar assistant' })).toHaveAttribute('aria-selected', 'true'));
    await user.keyboard('{ArrowDown}');
    const mail = screen.getByRole('option', { name: 'Mail assistant' });
    expect(mail).toHaveAttribute('aria-selected', 'true');
    expect(input).toHaveAttribute('aria-activedescendant', mail.id);
    expect(input).toHaveAttribute('aria-controls', screen.getByRole('listbox', { name: 'Console results' }).id);
    expect(input).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onSelect).toHaveBeenCalledExactlyOnceWith('mail');
    await user.click(screen.getByRole('option', { name: 'Unavailable assistant' }));
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('option', { name: 'Unavailable assistant' })).toHaveAttribute('aria-disabled', 'true');
  });
  it('announces loading outside the results popup and opens it when options arrive', async () => {
    const user = userEvent.setup();
    function AsyncResults({ loading }: { loading: boolean }) {
      return (
        <Command label="Search console">
          <CommandInput />
          <CommandList label="Console results">
            {!loading && <CommandItem value="calendar">Calendar assistant</CommandItem>}
          </CommandList>
          {loading && <CommandLoading label="Loading agents">Loading agents…</CommandLoading>}
        </Command>
      );
    }
    const view = render(<AsyncResults loading />);
    const input = screen.getByRole('combobox', { name: 'Search console' });
    await user.click(input);
    expect(screen.getByRole('status')).toBeVisible();
    expect(screen.getByRole('progressbar', { name: 'Loading agents' })).toBeVisible();
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(input).toHaveAttribute('aria-expanded', 'false');
    view.rerender(<AsyncResults loading={false} />);
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    expect(screen.getByRole('listbox', { name: 'Console results' })).toBeVisible();
    expect(screen.getByRole('option', { name: 'Calendar assistant' })).toBeVisible();
    expect(input).toHaveAttribute('aria-expanded', 'true');
    expect(input).toHaveFocus();
  });
});
