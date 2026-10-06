/*
 * Adapted from shadcn/ui (https://github.com/shadcn-ui/ui), MIT License.
 * Copyright (c) 2023 shadcn
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
import { forwardRef } from 'react';
import type { ComponentProps, ComponentPropsWithoutRef } from 'react';
import { Command as CommandPrimitive, useCommandState } from 'cmdk';
import { Search } from 'lucide-react';
import { cn } from '@design-system/utils/cn';
import './Command.css';

export type CommandProps = ComponentProps<typeof CommandPrimitive> & { label: string };
export type CommandInputProps = Omit<ComponentProps<typeof CommandPrimitive.Input>, 'asChild' | 'children'>;
export type CommandListProps = ComponentProps<typeof CommandPrimitive.List> & { label: string };
export type CommandEmptyProps = ComponentProps<typeof CommandPrimitive.Empty>;
export type CommandGroupProps = ComponentProps<typeof CommandPrimitive.Group>;
export type CommandItemProps = ComponentProps<typeof CommandPrimitive.Item>;
export type CommandLoadingProps = ComponentProps<typeof CommandPrimitive.Loading> & { label: string };

export function Command({ className, ...props }: CommandProps) {
  return (
    <CommandPrimitive
      data-slot="command"
      className={cn('flex w-full flex-col overflow-hidden rounded-md border border-border bg-popover text-popover-foreground', className)}
      {...props}
    />
  );
}

const NativeCommandInput = forwardRef<HTMLInputElement, ComponentPropsWithoutRef<'input'>>(function NativeCommandInput(props, ref) {
  // Keep cmdk's generated list ID while reflecting whether its popup has results.
  const hasResults = useCommandState((state) => state.filtered.count > 0);
  const selectedId = useCommandState((state) => state.selectedItemId);
  return <input {...props} ref={ref} role="combobox" aria-controls={props['aria-controls']}
    aria-expanded={hasResults} aria-activedescendant={hasResults ? selectedId : undefined} />;
});

export function CommandInput({ className, ...props }: CommandInputProps) {
  return (
    <div data-slot="command-input-wrapper" className="flex items-center gap-2 border-b border-border-subtle px-3 py-1">
      <Search aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
      <CommandPrimitive.Input
        data-slot="command-input"
        className={cn(
          'min-h-11 min-w-0 w-full rounded-lg border border-transparent bg-transparent px-2 py-2 text-sm text-popover-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring disabled:cursor-not-allowed disabled:text-muted-foreground aria-invalid:border-status-danger-foreground',
          className,
        )}
        {...props}
        asChild
      >
        <NativeCommandInput />
      </CommandPrimitive.Input>
    </div>
  );
}

export function CommandList({ className, ...props }: CommandListProps) {
  // Keep items mounted for cmdk registration, but close the popup when no options match.
  const hasResults = useCommandState((state) => state.filtered.count > 0);
  return (
    <CommandPrimitive.List
      data-slot="command-list"
      className={cn('max-h-72 scroll-py-1 overflow-x-hidden overflow-y-auto p-1', className)}
      {...props}
      hidden={!hasResults}
    />
  );
}

/** Render beside CommandList, not inside its option-only popup. */
export function CommandEmpty({ className, ...props }: CommandEmptyProps) {
  return (
    <div role="status" aria-live="polite" aria-atomic="true">
      <CommandPrimitive.Empty
        data-slot="command-empty"
        className={cn('px-3 py-6 text-center text-sm text-muted-foreground', className)}
        {...props}
      />
    </div>
  );
}

export function CommandGroup({ className, ...props }: CommandGroupProps) {
  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      className={cn('p-1 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground', className)}
      {...props}
    />
  );
}

export function CommandItem({ className, ...props }: CommandItemProps) {
  return (
    <CommandPrimitive.Item
      data-slot="command-item"
      className={cn(
        'ds-command-item relative flex min-h-11 cursor-default items-center gap-2 rounded-sm px-2 py-2 text-sm break-words outline-none select-none data-[disabled=true]:cursor-not-allowed data-[disabled=true]:text-muted-foreground data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground data-[selected=true]:ring-1 data-[selected=true]:ring-inset data-[selected=true]:ring-ring [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
        className,
      )}
      {...props}
    />
  );
}

export function CommandSeparator({ className, ...props }: ComponentProps<typeof CommandPrimitive.Separator>) {
  return <CommandPrimitive.Separator data-slot="command-separator" className={cn('my-1 h-px bg-border-subtle', className)} {...props} aria-hidden="true" />;
}

export function CommandShortcut({ className, ...props }: ComponentProps<'span'>) {
  return <span data-slot="command-shortcut" className={cn('ml-auto shrink-0 font-mono text-xs text-muted-foreground', className)} {...props} />;
}

/** Render beside CommandList; the loading announcement is not a selectable result. */
export function CommandLoading({ className, ...props }: CommandLoadingProps) {
  return (
    <div role="status" aria-live="polite" aria-atomic="true">
      <CommandPrimitive.Loading data-slot="command-loading" className={cn('px-3 py-6 text-center text-sm text-muted-foreground', className)} {...props} />
    </div>
  );
}
