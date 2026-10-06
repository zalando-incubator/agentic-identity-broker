// Adapted from shadcn/ui (MIT). Copyright (c) 2023 shadcn. See ../Input/LICENSE.
import type { ComponentProps } from 'react';
import { Select as SelectPrimitive } from 'radix-ui';
import { Check, ChevronDown, ChevronUp } from 'lucide-react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils/cn';
import { useInertBackgroundRef } from '@design-system/utils/inertBackground';

export type SelectProps = ComponentProps<typeof SelectPrimitive.Root>;
export const Select = SelectPrimitive.Root;
export const SelectGroup = SelectPrimitive.Group;
export const SelectValue = SelectPrimitive.Value;

const triggerVariants = cva(
  'flex w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-border-control bg-background px-3 text-sm text-foreground outline-none transition-colors duration-(--motion-control) ease-(--motion-ease) hover:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background data-[placeholder]:text-muted-foreground disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground aria-invalid:border-status-danger-foreground [&>span]:truncate',
  {
    variants: { size: { default: 'h-9', sm: 'h-8' } },
    defaultVariants: { size: 'default' },
  },
);

export type SelectTriggerProps = ComponentProps<typeof SelectPrimitive.Trigger> & VariantProps<typeof triggerVariants>;

export function SelectTrigger({ className, size, children, ...props }: SelectTriggerProps) {
  return (
    <SelectPrimitive.Trigger {...props} data-slot="select-trigger" className={cn(triggerVariants({ size }), className)}>
      {children}
      <SelectPrimitive.Icon asChild>
        <ChevronDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  );
}

export function SelectContent({
  className,
  children,
  position = 'popper',
  sideOffset = 4,
  ref,
  ...props
}: ComponentProps<typeof SelectPrimitive.Content>) {
  const contentRef = useInertBackgroundRef(ref);
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        {...props}
        ref={contentRef}
        position={position}
        sideOffset={sideOffset}
        data-slot="select-content"
        className={cn(
          'relative shadow-overlay z-50 max-h-(--radix-select-content-available-height) min-w-32 overflow-hidden rounded-md border border-border bg-popover text-popover-foreground',
          'transition-opacity duration-(--motion-overlay) ease-(--motion-ease) starting:opacity-0',
          className,
        )}
      >
        <SelectScrollUpButton />
        <SelectPrimitive.Viewport className={cn('p-1', position === 'popper' && 'min-w-(--radix-select-trigger-width)')}>
          {children}
        </SelectPrimitive.Viewport>
        <SelectScrollDownButton />
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  );
}

export function SelectLabel({ className, ...props }: ComponentProps<typeof SelectPrimitive.Label>) {
  return <SelectPrimitive.Label {...props} data-slot="select-label" className={cn('px-2 py-1.5 text-xs font-medium text-muted-foreground', className)} />;
}

export function SelectItem({ className, children, ...props }: ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      {...props}
      data-slot="select-item"
      className={cn(
        'relative flex min-h-9 w-full cursor-default select-none items-center rounded-sm py-1.5 pl-2 pr-8 text-sm outline-none',
        'focus:bg-accent focus:text-accent-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring data-[disabled]:pointer-events-none data-[disabled]:text-muted-foreground',
        className,
      )}
    >
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
      <span className="absolute right-2 flex size-4 items-center justify-center">
        <SelectPrimitive.ItemIndicator><Check aria-hidden="true" className="size-4" /></SelectPrimitive.ItemIndicator>
      </span>
    </SelectPrimitive.Item>
  );
}

export function SelectSeparator({ className, ...props }: ComponentProps<typeof SelectPrimitive.Separator>) {
  return <SelectPrimitive.Separator {...props} data-slot="select-separator" className={cn('-mx-1 my-1 h-px bg-border-subtle', className)} />;
}

export function SelectScrollUpButton({ className, ...props }: ComponentProps<typeof SelectPrimitive.ScrollUpButton>) {
  return (
    <SelectPrimitive.ScrollUpButton {...props} data-slot="select-scroll-up-button" className={cn('flex items-center justify-center py-1', className)}>
      <ChevronUp aria-hidden="true" className="size-4" />
    </SelectPrimitive.ScrollUpButton>
  );
}

export function SelectScrollDownButton({ className, ...props }: ComponentProps<typeof SelectPrimitive.ScrollDownButton>) {
  return (
    <SelectPrimitive.ScrollDownButton {...props} data-slot="select-scroll-down-button" className={cn('flex items-center justify-center py-1', className)}>
      <ChevronDown aria-hidden="true" className="size-4" />
    </SelectPrimitive.ScrollDownButton>
  );
}
