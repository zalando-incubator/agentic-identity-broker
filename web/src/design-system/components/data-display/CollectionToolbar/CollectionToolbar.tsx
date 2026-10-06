import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactElement, type Ref } from 'react';
import { ArrowDownUp, Check, LayoutGrid, List, ListFilter, Search, X } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';
import { Input } from '@design-system/components/inputs/Input';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@design-system/components/overlays/DropdownMenu';
import { Popover, PopoverContent, PopoverTrigger } from '@design-system/components/overlays/Popover';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@design-system/components/overlays/Tooltip';
import { cn } from '@design-system/utils/cn';

const views = ['grid', 'list'] as const;
const selectedControl = 'bg-primary-soft text-primary-soft-foreground hover:bg-primary-soft hover:text-primary-soft-foreground';

export interface CollectionToolbarProps {
  search: string;
  onSearchChange: (value: string) => void;
  sort?: string;
  onSortChange?: (value: string) => void;
  sortOptions?: readonly { value: string; label: string }[];
  view?: 'grid' | 'list';
  onViewChange?: (view: 'grid' | 'list') => void;
  filters?: readonly { value: string; label: string }[];
  filter?: string;
  onFilterChange?: (value: string) => void;
  labels: { search: string; sort: string; filter: string; grid: string; list: string; clearSearch: string };
  searchInputRef?: Ref<HTMLInputElement>;
}

function IconTooltip({ label, children }: { label: string; children: ReactElement }) {
  return <Tooltip><TooltipTrigger asChild>{children}</TooltipTrigger><TooltipContent>{label}</TooltipContent></Tooltip>;
}

export function CollectionToolbar({ search, onSearchChange, sort, onSortChange, sortOptions, view, onViewChange, filters, filter, onFilterChange, labels, searchInputRef }: CollectionToolbarProps) {
  const [expanded, setExpanded] = useState(false);
  const showSearch = expanded;
  const input = useRef<HTMLInputElement>(null);
  const searchButton = useRef<HTMLButtonElement>(null);
  const focusInput = useRef(false);
  const focusButton = useRef(false);
  const setInputRef = useCallback((node: HTMLInputElement | null) => {
    input.current = node;
    if (typeof searchInputRef === 'function') searchInputRef(node);
    else if (searchInputRef) searchInputRef.current = node;
  }, [searchInputRef]);

  useEffect(() => {
    if (showSearch && focusInput.current) {
      input.current?.focus();
      focusInput.current = false;
    }
    if (!showSearch && focusButton.current) {
      searchButton.current?.focus();
      focusButton.current = false;
    }
  }, [showSearch]);

  useEffect(() => {
    const onShortcut = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || event.repeat) return;
      const target = event.target;
      if (target instanceof Element && target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"], [role="combobox"]')) return;
      event.preventDefault();
      if (showSearch) input.current?.focus();
      else {
        focusInput.current = true;
        setExpanded(true);
      }
    };
    window.addEventListener('keydown', onShortcut);
    return () => window.removeEventListener('keydown', onShortcut);
  }, [showSearch]);

  const onSearchKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key !== 'Escape') return;
    event.preventDefault();
    event.stopPropagation();
    focusButton.current = true;
    onSearchChange('');
    setExpanded(false);
  };

  const activeFilter = filters?.find(option => option.value === filter);
  const searchLabel = search ? `${labels.search}: ${search}` : labels.search;
  return <TooltipProvider><div data-slot="collection-toolbar" className="flex max-w-full flex-wrap items-center justify-end gap-1">
    {showSearch ? <div className="relative w-60 max-w-full" onBlur={event => {
      if (!event.currentTarget.contains(event.relatedTarget)) setExpanded(false);
    }}>
      <Search aria-hidden="true" className="pointer-events-none absolute left-2 top-2 size-4 text-muted-foreground" />
      <Input ref={setInputRef} aria-label={labels.search} placeholder={labels.search} value={search} onChange={event => onSearchChange(event.target.value)}
        onKeyDown={onSearchKeyDown} className={cn('h-8 pl-8', search && 'pr-8')} />
      {search && <IconTooltip label={labels.clearSearch}><Button aria-label={labels.clearSearch} variant="ghost" size="sm" className="absolute right-0 top-0 w-8 px-0" onKeyDown={onSearchKeyDown} onClick={() => { onSearchChange(''); input.current?.focus(); }}><X aria-hidden="true" /></Button></IconTooltip>}
    </div> : <IconTooltip label={searchLabel}><Button ref={searchButton} aria-label={searchLabel} variant="ghost" size="sm" className={cn('w-8 px-0', search && selectedControl)} onClick={() => { focusInput.current = true; setExpanded(true); }}><Search aria-hidden="true" /></Button></IconTooltip>}

    {sortOptions && sortOptions.length > 0 && sort !== undefined && onSortChange && <DropdownMenu>
      <IconTooltip label={labels.sort}><DropdownMenuTrigger asChild><Button aria-label={labels.sort} variant="ghost" size="sm" className={cn('w-8 px-0', sort !== sortOptions[0]?.value && selectedControl)}><ArrowDownUp aria-hidden="true" /></Button></DropdownMenuTrigger></IconTooltip>
      <DropdownMenuContent align="end" aria-label={labels.sort}>
        {sortOptions.map(option => <DropdownMenuItem key={option.value} role="menuitemradio" aria-checked={sort === option.value} onSelect={() => onSortChange(option.value)}>
          <Check aria-hidden="true" className={cn('size-4', sort !== option.value && 'invisible')} />{option.label}
        </DropdownMenuItem>)}
      </DropdownMenuContent>
    </DropdownMenu>}

    {filters && filters.length > 0 && onFilterChange && <Popover>
      <IconTooltip label={labels.filter}><PopoverTrigger asChild><Button aria-label={activeFilter ? `${labels.filter}: ${activeFilter.label}` : labels.filter} variant="ghost" size="sm" className={cn('w-8 px-0', activeFilter && selectedControl)}><ListFilter aria-hidden="true" /></Button></PopoverTrigger></IconTooltip>
      <PopoverContent align="end" aria-label={labels.filter} className="w-56 p-2">
        <div role="group" aria-label={labels.filter} className="flex flex-col gap-1">
          {filters.map(option => <Button key={option.value} variant="outline" size="sm" aria-pressed={filter === option.value}
            onClick={() => onFilterChange(filter === option.value ? '' : option.value)} className={cn('w-full justify-start px-2', filter === option.value && selectedControl)}>
            <Check aria-hidden="true" className={cn('size-4', filter !== option.value && 'invisible')} />{option.label}
          </Button>)}
        </div>
      </PopoverContent>
    </Popover>}

    {view && onViewChange && <div role="group" aria-label={`${labels.grid} / ${labels.list}`} className="flex rounded-lg border border-border p-px">
      {views.map(option => <IconTooltip key={option} label={labels[option]}><Button aria-label={labels[option]} aria-pressed={view === option} variant="ghost" size="sm"
        className={cn('w-8 px-0', view === option && selectedControl)} onClick={() => onViewChange(option)}>
        {option === 'grid' ? <LayoutGrid aria-hidden="true" /> : <List aria-hidden="true" />}
      </Button></IconTooltip>)}
    </div>}
  </div></TooltipProvider>;
}
