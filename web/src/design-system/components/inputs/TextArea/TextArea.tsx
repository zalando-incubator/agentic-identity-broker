// Adapted from shadcn/ui (MIT). Copyright (c) 2023 shadcn. See ../Input/LICENSE.
import { useId, type ComponentProps, type ReactNode } from 'react';
import { cn } from '@design-system/utils/cn';

export type TextAreaProps = ComponentProps<'textarea'> & {
  label?: ReactNode;
  error?: ReactNode;
  description?: ReactNode;
};

export function TextArea({
  className,
  id: suppliedId,
  label,
  error,
  description,
  'aria-describedby': describedBy,
  ...props
}: TextAreaProps) {
  const generatedId = useId();
  const id = suppliedId ?? generatedId;
  const messageId = error ? `${id}-error` : description ? `${id}-description` : undefined;
  const textarea = (
    <textarea
      {...props}
      id={id}
      data-slot="textarea"
      aria-invalid={error ? true : props['aria-invalid']}
      aria-describedby={[describedBy, messageId].filter(Boolean).join(' ') || undefined}
      className={cn(
        'flex min-h-24 w-full min-w-0 resize-y rounded-lg border border-border-control bg-background px-3 py-2 text-base text-foreground outline-none placeholder:text-muted-foreground md:text-sm',
        'transition-colors duration-(--motion-control) ease-(--motion-ease) hover:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        'disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground aria-invalid:border-status-danger-foreground',
        className,
      )}
    />
  );

  if (!label && !error && !description) return textarea;

  return (
    <div className="w-full space-y-2">
      {label && <label htmlFor={id} className="block text-sm font-medium text-foreground">{label}</label>}
      {textarea}
      {error ? (
        <p id={messageId} role="alert" className="text-sm text-status-danger-foreground">{error}</p>
      ) : description ? (
        <p id={messageId} className="text-sm text-muted-foreground">{description}</p>
      ) : null}
    </div>
  );
}
