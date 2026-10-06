// Adapted from shadcn/ui (MIT). Copyright (c) 2023 shadcn. See ../Input/LICENSE.
import { useId, type ComponentProps, type ReactNode } from 'react';
import { Checkbox as CheckboxPrimitive } from 'radix-ui';
import { Check, Minus } from 'lucide-react';
import { cn } from '@design-system/utils/cn';

export type CheckboxProps = ComponentProps<typeof CheckboxPrimitive.Root> & {
  label?: ReactNode;
  description?: ReactNode;
  error?: ReactNode;
};

export function Checkbox({
  className,
  id: suppliedId,
  label,
  description,
  error,
  'aria-describedby': describedBy,
  ...props
}: CheckboxProps) {
  const generatedId = useId();
  const id = suppliedId ?? generatedId;
  const messageId = error ? `${id}-error` : description ? `${id}-description` : undefined;
  const checkbox = (
    <CheckboxPrimitive.Root
      {...props}
      id={id}
      data-slot="checkbox"
      aria-invalid={error ? true : props['aria-invalid']}
      aria-describedby={[describedBy, messageId].filter(Boolean).join(' ') || undefined}
      className={cn(
        'group/checkbox flex size-6 shrink-0 items-center justify-center rounded-sm border border-border-control bg-background text-foreground outline-none transition-colors duration-(--motion-control) ease-(--motion-ease)',
        'hover:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:data-[state=unchecked]:bg-muted',
        'data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:border-primary data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground',
        'aria-invalid:border-status-danger-foreground',
        className,
      )}
    >
      <CheckboxPrimitive.Indicator data-slot="checkbox-indicator" className="flex items-center justify-center">
        <Check aria-hidden="true" className="size-4 group-data-[state=indeterminate]/checkbox:hidden" />
        <Minus aria-hidden="true" className="hidden size-4 group-data-[state=indeterminate]/checkbox:block" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );

  if (!label && !error && !description) return checkbox;

  return (
    <div className="flex items-start gap-3">
      {checkbox}
      <div className="space-y-1">
        {label && <label htmlFor={id} className="block text-sm font-medium text-foreground">{label}</label>}
        {error ? (
          <p id={messageId} role="alert" className="text-sm text-status-danger-foreground">{error}</p>
        ) : description ? (
          <p id={messageId} className="text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
    </div>
  );
}
