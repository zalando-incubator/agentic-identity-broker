// Adapted from shadcn/ui (MIT). Copyright (c) 2023 shadcn. See ../Input/LICENSE.
import type { ComponentProps } from 'react';
import { RadioGroup as RadioPrimitive } from 'radix-ui';
import { Circle } from 'lucide-react';
import { cn } from '@design-system/utils/cn';

export type RadioGroupProps = ComponentProps<typeof RadioPrimitive.Root>;
export type RadioGroupItemProps = ComponentProps<typeof RadioPrimitive.Item>;

export function RadioGroup({ className, ...props }: RadioGroupProps) {
  return (
    <RadioPrimitive.Root
      {...props}
      data-slot="radio-group"
      className={cn('group/radio-group grid gap-3', className)}
    />
  );
}

export function RadioGroupItem({ className, ...props }: RadioGroupItemProps) {
  return (
    <RadioPrimitive.Item
      {...props}
      data-slot="radio-group-item"
      className={cn(
        'flex size-6 shrink-0 items-center justify-center rounded-full border border-input bg-background text-primary outline-none transition-colors duration-(--motion-control) ease-(--motion-ease)',
        'hover:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        'disabled:cursor-not-allowed disabled:bg-muted aria-invalid:border-destructive group-aria-invalid/radio-group:border-destructive motion-reduce:transition-none',
        className,
      )}
    >
      <RadioPrimitive.Indicator data-slot="radio-group-indicator" className="flex items-center justify-center">
        <Circle aria-hidden="true" className="size-3 fill-current" />
      </RadioPrimitive.Indicator>
    </RadioPrimitive.Item>
  );
}
