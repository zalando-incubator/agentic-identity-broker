import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { expect, fn, userEvent, waitFor, within } from 'storybook/test';
import { CollectionToolbar, type CollectionToolbarProps } from './CollectionToolbar';

function ToolbarDemo(args: CollectionToolbarProps) {
  const [search, setSearch] = useState(args.search);
  const [sort, setSort] = useState(args.sort);
  const [filter, setFilter] = useState(args.filter ?? '');
  const [view, setView] = useState(args.view);
  return <div className="max-w-xl space-y-4" data-testid="toolbar-demo">
    <CollectionToolbar {...args} search={search} sort={sort} filter={filter} view={view}
      onSearchChange={value => { setSearch(value); args.onSearchChange(value); }}
      onSortChange={value => { setSort(value); args.onSortChange(value); }}
      onFilterChange={args.onFilterChange && (value => { setFilter(value); args.onFilterChange?.(value); })}
      onViewChange={args.onViewChange && (value => { setView(value); args.onViewChange?.(value); })} />
    <p className="text-sm text-muted-foreground" data-testid="toolbar-state">{search || 'All'} · {sort} · {filter || 'All'} · {view}</p>
  </div>;
}

const meta = {
  title: 'Design System/Data Display/CollectionToolbar',
  component: CollectionToolbar,
  parameters: { layout: 'padded', a11y: { test: 'error' } },
  tags: ['autodocs'],
  render: args => <ToolbarDemo {...args} />,
  args: {
    search: '', onSearchChange: fn(),
    sort: 'name', onSortChange: fn(),
    sortOptions: [{ value: 'name', label: 'Name' }, { value: 'recent', label: 'Recently changed' }, { value: 'expiring', label: 'Expiring soonest' }],
    view: 'grid', onViewChange: fn(),
    filters: [{ value: 'active', label: 'Active' }, { value: 'expired', label: 'Expired' }],
    filter: '', onFilterChange: fn(),
    labels: { search: 'Search connections', clearSearch: 'Clear search', sort: 'Sort connections', filter: 'Connection state', grid: 'Grid view', list: 'List view' },
  },
} satisfies Meta<typeof CollectionToolbar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Dark: Story = { globals: { theme: 'dark' } };
export const SearchExpanded: Story = { args: { search: 'Calendar' }, play: async ({ canvasElement }) => { await userEvent.click(within(canvasElement).getByRole('button', { name: 'Search connections: Calendar', exact: true })); } };
export const SearchBlurred: Story = {
  args: { search: 'Calendar' },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Search connections: Calendar', exact: true }));
    const input = canvas.getByRole('textbox', { name: 'Search connections' });
    await userEvent.click(input);
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'Clear search' })).toHaveFocus();
    await expect(input).toHaveValue('Calendar');
    await userEvent.tab();
    await waitFor(() => expect(canvas.queryByRole('textbox', { name: 'Search connections' })).not.toBeInTheDocument());
    const icon = canvas.getByRole('button', { name: 'Search connections: Calendar' });
    await userEvent.hover(icon);
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent('Search connections: Calendar'));
    await userEvent.unhover(icon);
    await userEvent.click(canvas.getByTestId('toolbar-state'));
  },
};
export const SearchKeyboardClear: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Search connections' }));
    const input = canvas.getByRole('textbox', { name: 'Search connections' });
    await userEvent.type(input, 'calendar');
    await userEvent.tab();
    await expect(canvas.getByRole('button', { name: 'Clear search' })).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    await expect(input).toHaveValue('');
    await expect(input).toHaveFocus();
    await userEvent.click(canvas.getByTestId('toolbar-state'));
    await expect(canvas.queryByRole('textbox', { name: 'Search connections' })).not.toBeInTheDocument();
  },
};
export const SearchShortcut: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.keyboard('/');
    const input = await canvas.findByRole('textbox', { name: 'Search connections' });
    await waitFor(() => expect(input).toHaveFocus());
    await userEvent.type(input, 'calendar');
    await waitFor(() => expect(args.onSearchChange).toHaveBeenCalledWith('calendar'));
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(canvas.getByRole('button', { name: 'Search connections' })).toHaveFocus());
    await waitFor(() => expect(canvas.queryByRole('textbox', { name: 'Search connections' })).not.toBeInTheDocument());
  },
};
export const SortOpen: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const height = canvas.getByTestId('toolbar-demo').getBoundingClientRect().height;
    await userEvent.click(canvas.getByRole('button', { name: 'Sort connections' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('menuitemradio', { name: 'Name' })).toHaveAttribute('aria-checked', 'true'));
    await expect(canvas.getByTestId('toolbar-demo').getBoundingClientRect().height).toBe(height);
  },
};
export const FilterSelected: Story = {
  args: { filter: 'active' },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: 'Connection state: Active' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('dialog', { name: 'Connection state' })).toBeVisible());
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('button', { name: 'Active' })).toHaveAttribute('aria-pressed', 'true'));
  },
};
export const ListSelected: Story = { args: { view: 'list' } };
export const NoFacet: Story = {
  args: { filters: undefined, onFilterChange: undefined },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole('button', { name: 'Connection state' })).not.toBeInTheDocument();
  },
};
export const Hover: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole('button', { name: 'Sort connections' }));
    await waitFor(() => expect(within(canvasElement.ownerDocument.body).getByRole('tooltip')).toHaveTextContent('Sort connections'));
  },
};
export const FocusVisible: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole('button', { name: 'Search connections' })).toHaveFocus();
  },
};
