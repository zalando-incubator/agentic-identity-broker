import { useRef, useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { CollectionToolbar } from './CollectionToolbar';

const labels = { search: 'Search agents', clearSearch: 'Clear search', sort: 'Sort agents', filter: 'Filter agents', grid: 'Grid view', list: 'List view' };
const sortOptions = [{ value: 'name', label: 'Name' }, { value: 'recent', label: 'Recently changed' }] as const;
const filters = [{ value: 'active', label: 'Active' }, { value: 'expired', label: 'Expired' }] as const;

function Toolbar({ withOptions = true }: { withOptions?: boolean }) {
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState('name');
  const [filter, setFilter] = useState('');
  const [view, setView] = useState<'grid' | 'list'>('grid');
  const inputRef = useRef<HTMLInputElement>(null);
  return <>
    <CollectionToolbar search={search} onSearchChange={setSearch} sort={sort} onSortChange={setSort} sortOptions={withOptions ? sortOptions : []}
      view={withOptions ? view : undefined} onViewChange={withOptions ? setView : undefined}
      filters={withOptions ? filters : undefined} filter={filter} onFilterChange={withOptions ? setFilter : undefined}
      labels={labels} searchInputRef={inputRef} />
    <output data-testid="state">{search}|{sort}|{filter}|{view}</output>
    <output data-testid="ref">{inputRef.current?.getAttribute('aria-label') ?? ''}</output>
  </>;
}

describe('CollectionToolbar', () => {
  it('opens search with /, ignores editable targets, and clears and collapses with Escape', async () => {
    const user = userEvent.setup();
    render(<><input aria-label="Other form" /><Toolbar /></>);
    await user.click(screen.getByRole('textbox', { name: 'Other form' }));
    await user.keyboard('/');
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    await user.tab();
    await user.keyboard('/');
    const search = screen.getByRole('textbox', { name: 'Search agents' });
    expect(search).toHaveFocus();
    await user.type(search, 'alpha');
    expect(screen.getByTestId('state')).toHaveTextContent('alpha|name||grid');
    expect(screen.getByTestId('ref')).toHaveTextContent('Search agents');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search agents' })).toHaveFocus();
    expect(screen.getByTestId('state')).toHaveTextContent(/^\|name\|\|grid$/);
  });

  it('leaves the slash shortcut to a content-editable field', async () => {
    const user = userEvent.setup();
    render(<><div role="textbox" aria-label="Inline editor" contentEditable suppressContentEditableWarning /><Toolbar /></>);
    await user.click(screen.getByRole('textbox', { name: 'Inline editor' }));
    await user.keyboard('/');
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
  });

  it('collapses an empty search on outside focus without moving focus back to the icon', async () => {
    const user = userEvent.setup();
    render(<><Toolbar /><button>Outside</button></>);
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'Outside' }));
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Outside' })).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Search agents' })).toBeVisible();
  });

  it('keeps a typed query when blurred and reveals it on the icon, slash, and explicit reopen', async () => {
    const user = userEvent.setup();
    render(<><Toolbar /><button>Outside</button></>);
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    await user.type(screen.getByRole('textbox', { name: 'Search agents' }), 'beta');
    await user.click(screen.getByRole('button', { name: 'Outside' }));
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Outside' })).toHaveFocus();
    expect(screen.getByTestId('state')).toHaveTextContent('beta|name||grid');
    const activeSearch = screen.getByRole('button', { name: 'Search agents: beta' });
    await user.hover(activeSearch);
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Search agents: beta');
    await user.unhover(activeSearch);
    await user.keyboard('/');
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveValue('beta');
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'Outside' }));
    await user.click(screen.getByRole('button', { name: 'Search agents: beta' }));
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveValue('beta');
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveFocus();
  });

  it('keeps search expanded while tabbing to Clear and supports keyboard clearing', async () => {
    const user = userEvent.setup();
    render(<Toolbar />);
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    const search = screen.getByRole('textbox', { name: 'Search agents' });
    await user.type(search, 'alpha');
    await user.tab();
    expect(screen.getByRole('button', { name: 'Clear search' })).toHaveFocus();
    expect(search).toHaveValue('alpha');
    await user.keyboard('{Enter}');
    expect(search).toHaveValue('');
    expect(search).toHaveFocus();
    expect(screen.getByTestId('state')).toHaveTextContent(/^\|name\|\|grid$/);
    await user.tab();
    expect(screen.getByRole('button', { name: 'Sort agents' })).toHaveFocus();
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
  });

  it('clears on Escape from the Clear button and restores focus to the search icon', async () => {
    const user = userEvent.setup();
    render(<Toolbar />);
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    await user.type(screen.getByRole('textbox', { name: 'Search agents' }), 'alpha');
    await user.tab();
    expect(screen.getByRole('button', { name: 'Clear search' })).toHaveFocus();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('textbox', { name: 'Search agents' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search agents' })).toHaveFocus();
    expect(screen.getByTestId('state')).toHaveTextContent(/^\|name\|\|grid$/);
  });

  it('changes exclusive sort and a real facet, then clears the active facet', async () => {
    const user = userEvent.setup();
    render(<Toolbar />);
    await user.click(screen.getByRole('button', { name: 'Sort agents' }));
    const menu = screen.getByRole('menu');
    expect(within(menu).getByRole('menuitemradio', { name: 'Name' })).toHaveAttribute('aria-checked', 'true');
    await user.click(within(menu).getByRole('menuitemradio', { name: 'Recently changed' }));
    expect(screen.getByTestId('state')).toHaveTextContent('|recent||grid');
    await user.click(screen.getByRole('button', { name: 'Sort agents' }));
    expect(screen.getByRole('menuitemradio', { name: 'Recently changed' })).toHaveAttribute('aria-checked', 'true');
    await user.keyboard('{Escape}');
    await user.click(screen.getByRole('button', { name: 'Filter agents' }));
    const active = screen.getByRole('button', { name: 'Active' });
    await user.click(active);
    expect(active).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByTestId('state')).toHaveTextContent('|recent|active|grid');
    await user.click(active);
    expect(screen.getByTestId('state')).toHaveTextContent('|recent||grid');
  });

  it('switches the pressed view and hides controls with no caller behavior', async () => {
    const user = userEvent.setup();
    const { rerender } = render(<Toolbar />);
    await user.click(screen.getByRole('button', { name: 'List view' }));
    expect(screen.getByRole('button', { name: 'List view' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Grid view' })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByTestId('state')).toHaveTextContent('|name||list');
    rerender(<Toolbar withOptions={false} />);
    expect(screen.queryByRole('button', { name: 'Filter agents' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sort agents' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'List view' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Search agents' }));
    await user.type(screen.getByRole('textbox', { name: 'Search agents' }), 'a');
    await user.click(screen.getByRole('button', { name: 'Clear search' }));
    expect(screen.getByRole('textbox', { name: 'Search agents' })).toHaveValue('');
  });

  it('renders only supplied copy and callbacks', () => {
    const onChange = vi.fn();
    render(<CollectionToolbar search="" onSearchChange={onChange} sort="" onSortChange={onChange} sortOptions={[]} labels={labels} />);
    expect(screen.getAllByRole('button')).toHaveLength(1);
  });
});
