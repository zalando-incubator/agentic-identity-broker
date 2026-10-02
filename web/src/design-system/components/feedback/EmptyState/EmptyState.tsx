import type { ComponentProps, ReactNode } from 'react';
import { cn } from '@design-system/utils';
import { Button } from '@design-system/components/primitives/Button';

export interface EmptyStateProps extends Omit<ComponentProps<'div'>, 'title'> {
  title: string;
  description?: string;
  icon?: ReactNode;
  wordmark?: ReactNode;
  primaryAction?: { label: string; onClick: () => void };
  secondaryAction?: { label: string; onClick: () => void };
}

export function EmptyState({ title, description, icon, wordmark, primaryAction, secondaryAction, children, className, ...props }: EmptyStateProps) {
  return (
    <div role="status" aria-live="polite" {...props} className={cn('flex flex-col items-center gap-4 rounded-lg border border-border bg-card p-6 text-center text-card-foreground', className)}>
      {wordmark}
      {icon && <div aria-hidden="true" className="text-muted-foreground [&>svg]:size-10">{icon}</div>}
      <h3 className="font-display text-xl font-semibold">{title}</h3>
      {description && <p className="max-w-md text-sm text-muted-foreground">{description}</p>}
      {(primaryAction || secondaryAction) && <div className="flex flex-wrap justify-center gap-3">
        {primaryAction && <Button onClick={primaryAction.onClick}>{primaryAction.label}</Button>}
        {secondaryAction && <Button variant="outline" onClick={secondaryAction.onClick}>{secondaryAction.label}</Button>}
      </div>}
      {children}
    </div>
  );
}

export default EmptyState;
