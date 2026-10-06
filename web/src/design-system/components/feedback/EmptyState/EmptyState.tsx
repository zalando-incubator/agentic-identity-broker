import type { ComponentProps, ReactNode } from 'react';
import { cn } from '@design-system/utils';
import { Button } from '@design-system/components/primitives/Button';

export interface EmptyStateProps extends Omit<ComponentProps<'div'>, 'title'> {
  title: string;
  size?: 'default' | 'compact';
  description?: string;
  icon?: ReactNode;
  primaryAction?: { label: string; onClick: () => void };
  secondaryAction?: { label: string; onClick: () => void };
}

export function EmptyState({ title, size = 'default', description, icon, primaryAction, secondaryAction, children, className, ...props }: EmptyStateProps) {
  return (
    <div role="status" aria-live="polite" {...props} className={cn('flex flex-col items-center rounded-xl border border-border-subtle bg-card text-center text-card-foreground', size === 'compact' ? 'gap-3 p-4' : 'gap-4 p-6', className)}>
      {icon && <div aria-hidden="true" data-animated-icon-host data-empty-state-illustration className="flex size-20 items-center justify-center rounded-2xl bg-primary-soft text-primary-soft-foreground [&>svg]:size-10">{icon}</div>}
      <h2 className={cn('font-display font-semibold', size === 'compact' ? 'text-base' : 'text-xl')}>{title}</h2>
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
