import type { ComponentProps, ReactNode } from 'react';
import { CircleAlert } from 'lucide-react';
import { cn } from '@design-system/utils';
import { Button } from '@design-system/components/primitives/Button';

type Retry = { onRetry: () => void; retryLabel: string; isRetrying?: boolean } | { onRetry?: never; retryLabel?: never; isRetrying?: never };
export type InlineErrorProps = Omit<ComponentProps<'div'>, 'children'> & Retry & {
  message: string;
  icon?: ReactNode;
  hideIcon?: boolean;
  errors?: string[];
  suggestions?: string[];
  helperText?: string;
  fieldLabel?: string;
};

export function InlineError({ message, icon, hideIcon, errors, suggestions, helperText, fieldLabel, onRetry, retryLabel, isRetrying, className, ...props }: InlineErrorProps) {
  const messages = errors?.length ? errors : [message];
  return (
    <div role="alert" aria-live="polite" aria-atomic="true" {...props} className={cn('space-y-2 text-sm text-destructive', className)}>
      {fieldLabel && <span className="sr-only">{fieldLabel}</span>}
      <div className="flex items-start gap-2">
        {!hideIcon && <span aria-hidden="true" className="mt-0.5 shrink-0">{icon ?? <CircleAlert className="size-4" />}</span>}
        {messages.length > 1 ? <ul className="list-inside list-disc space-y-1">{messages.map((error, index) => <li key={index}>{error}</li>)}</ul> : <p>{messages[0]}</p>}
      </div>
      {suggestions?.length ? <ul className="list-inside list-disc text-muted-foreground">{suggestions.map((suggestion, index) => <li key={index}>{suggestion}</li>)}</ul> : null}
      {helperText && <p className="text-muted-foreground">{helperText}</p>}
      {onRetry && <Button variant="outline" size="sm" onClick={onRetry} isLoading={isRetrying}>{retryLabel}</Button>}
    </div>
  );
}

export default InlineError;
