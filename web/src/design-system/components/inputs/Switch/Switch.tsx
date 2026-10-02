// Adapted from shadcn/ui (MIT). Copyright (c) 2023 shadcn. See ../Input/LICENSE.
import { useId, type ComponentProps, type ReactNode } from 'react';
import { Switch as SwitchPrimitive } from 'radix-ui';
import { cn } from '@design-system/utils/cn';

export type SwitchProps = ComponentProps<typeof SwitchPrimitive.Root> & {
  label?: ReactNode;
  description?: ReactNode;
  error?: ReactNode;
};

export function Switch({
  className,
  id: suppliedId,
  label,
  description,
  error,
  'aria-describedby': describedBy,
  ...props
}: SwitchProps) {
  const generatedId = useId();
  const id = suppliedId ?? generatedId;
  const messageId = error ? `${id}-error` : description ? `${id}-description` : undefined;
  const control = (
    <SwitchPrimitive.Root
      {...props}
      id={id}
      data-slot="switch"
      aria-invalid={error ? true : props['aria-invalid']}
      aria-describedby={[describedBy, messageId].filter(Boolean).join(' ') || undefined}
      className={cn(
        'inline-flex h-6 w-11 shrink-0 items-center rounded-full border border-input bg-muted outline-none transition-colors duration-(--motion-control) ease-(--motion-ease)',
        'data-[state=checked]:border-primary data-[state=checked]:bg-primary hover:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        'disabled:cursor-not-allowed aria-invalid:border-destructive motion-reduce:transition-none',
        className,
      )}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className="pointer-events-none block size-4 rounded-full bg-foreground transition-transform duration-(--motion-control) ease-(--motion-ease) data-[state=unchecked]:translate-x-0.5 data-[state=checked]:translate-x-6 data-[state=checked]:bg-primary-foreground motion-reduce:transition-none"
      />
    </SwitchPrimitive.Root>
  );

  if (!label && !error && !description) return control;

  return (
    <div className="flex items-start gap-3">
      {control}
      <div className="space-y-1">
        {label && <label htmlFor={id} className="block text-sm font-medium text-foreground">{label}</label>}
        {error ? (
          <p id={messageId} role="alert" className="text-sm text-destructive">{error}</p>
        ) : description ? (
          <p id={messageId} className="text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
    </div>
  );
}
