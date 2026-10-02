/*
 * Adapted from shadcn/ui (https://github.com/shadcn-ui/ui).
 * MIT License — Copyright (c) 2023 shadcn
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */
import type { ComponentProps } from 'react';
import { cn } from '@design-system/utils/cn';

// Explicit table roles retain semantics in browsers that otherwise remove them
// when the narrow layout changes native table display values.
export function Table({ className, ...props }: ComponentProps<'table'>) {
  return <div data-slot="table-container" className="relative min-w-0 w-full max-w-full">
    <table data-slot="table" role="table" className={cn('block w-full caption-bottom text-sm text-foreground sm:table sm:table-fixed', className)} {...props} />
  </div>;
}

export function TableHeader({ className, ...props }: ComponentProps<'thead'>) {
  return <thead data-slot="table-header" className={cn('sr-only sm:not-sr-only sm:table-header-group', className)} {...props} />;
}

export function TableBody({ className, ...props }: ComponentProps<'tbody'>) {
  return <tbody data-slot="table-body" className={cn('block sm:table-row-group [&>tr:last-child]:border-b-0', className)} {...props} />;
}

export function TableRow({ className, ...props }: ComponentProps<'tr'>) {
  return <tr data-slot="table-row" role="row" className={cn('grid min-w-0 border-b border-border-soft py-2 transition-colors duration-[var(--motion-feedback)] ease-[var(--motion-ease)] hover:bg-muted data-[state=selected]:bg-muted motion-reduce:transition-none sm:table-row sm:py-0', className)} {...props} />;
}

type TableHeadProps = ComponentProps<'th'> & { label?: string };
type TableCellProps = ComponentProps<'td'> & { label?: string };

export function TableHead({ className, scope = 'col', label, children, ...props }: TableHeadProps) {
  const rowHeader = scope === 'row' || scope === 'rowgroup';
  return <th data-slot="table-head" role={rowHeader ? 'rowheader' : 'columnheader'} scope={scope} className={cn(
    'min-w-0 px-3 py-2 text-left align-middle font-medium [overflow-wrap:anywhere]',
    rowHeader ? 'grid items-start gap-x-3 sm:table-cell' : 'text-muted-foreground',
    rowHeader && label && 'grid-cols-[minmax(0,1fr)_minmax(0,2fr)]',
    className,
  )} {...props}>
    {rowHeader && label && <span aria-hidden="true" className="min-w-0 text-muted-foreground sm:hidden">{label}</span>}
    <div className="min-w-0 [overflow-wrap:anywhere]">{children}</div>
  </th>;
}

/** Supply the column label for mobile rows; spanning status cells may omit it. */
export function TableCell({ className, label, children, ...props }: TableCellProps) {
  return <td data-slot="table-cell" className={cn('grid min-w-0 items-start gap-x-3 px-3 py-2 align-middle sm:table-cell', label && 'grid-cols-[minmax(0,1fr)_minmax(0,2fr)]', className)} {...props}>
    {label && <span aria-hidden="true" className="min-w-0 font-medium text-muted-foreground [overflow-wrap:anywhere] sm:hidden">{label}</span>}
    <div className="min-w-0 [overflow-wrap:anywhere]">{children}</div>
  </td>;
}

export function TableCaption({ className, ...props }: ComponentProps<'caption'>) {
  return <caption data-slot="table-caption" className={cn('block py-2 text-left text-sm text-muted-foreground sm:table-caption', className)} {...props} />;
}
