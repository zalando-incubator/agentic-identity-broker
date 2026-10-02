import { useState, type ComponentProps, type ReactNode } from 'react';
import { cva } from 'class-variance-authority';
import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from 'lucide-react';
import { cn } from '@design-system/utils';
import { Button } from '@design-system/components/primitives/Button';

const alertVariants = cva('relative flex gap-3 rounded-lg border border-border bg-card p-4 text-card-foreground', {
  variants: {
    variant: { info: '[&>svg]:text-info', success: '[&>svg]:text-success', warning: '[&>svg]:text-warning', error: '[&>svg]:text-destructive' },
    banner: { true: 'w-full rounded-none border-x-0', false: '' },
  },
  defaultVariants: { variant: 'info', banner: false },
});
const icons = { info: Info, success: CircleCheck, warning: TriangleAlert, error: CircleAlert };

type Dismissal = { dismissible: true; dismissLabel: string; onDismiss?: () => void } | { dismissible?: false; dismissLabel?: never; onDismiss?: never };
export type AlertProps = Omit<ComponentProps<'div'>, 'title'> & Dismissal & {
  variant?: 'info' | 'success' | 'warning' | 'error';
  icon?: ReactNode;
  title?: ReactNode;
  action?: { label: string; onClick: () => void };
  banner?: boolean;
  hideIcon?: boolean;
};

export function Alert({ variant = 'info', icon, title, dismissible, dismissLabel, onDismiss, action, banner, hideIcon, className, children, ...props }: AlertProps) {
  const [dismissed, setDismissed] = useState(false);
  if (dismissed) return null;
  const Icon = icons[variant];
  return (
    <div role={variant === 'error' ? 'alert' : 'status'} aria-live={variant === 'error' ? 'assertive' : 'polite'} aria-atomic="true" {...props} data-slot="alert" className={cn(alertVariants({ variant, banner }), className)}>
      {!hideIcon && (icon ? <span aria-hidden="true" className="shrink-0">{icon}</span> : <Icon aria-hidden="true" className="mt-0.5 size-5 shrink-0" />)}
      <div className="min-w-0 flex-1 space-y-2">
        {title && <AlertTitle>{title}</AlertTitle>}
        {children && <AlertDescription>{children}</AlertDescription>}
        {action && <Button variant="outline" size="sm" onClick={action.onClick}>{action.label}</Button>}
      </div>
      {dismissible && <Button variant="ghost" size="icon" aria-label={dismissLabel} className="shrink-0" onClick={() => { setDismissed(true); onDismiss?.(); }}><X aria-hidden="true" className="size-4" /></Button>}
    </div>
  );
}

export function AlertTitle({ className, children, ...props }: ComponentProps<'h3'>) {
  return <h3 {...props} data-slot="alert-title" className={cn('font-display font-semibold leading-tight text-foreground', className)}>{children}</h3>;
}

export function AlertDescription({ className, ...props }: ComponentProps<'div'>) {
  return <div {...props} data-slot="alert-description" className={cn('text-sm leading-relaxed text-muted-foreground', className)} />;
}

export default Alert;
