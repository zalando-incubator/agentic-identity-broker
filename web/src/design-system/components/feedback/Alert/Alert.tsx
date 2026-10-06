import { useState, type ComponentProps, type ReactNode } from 'react';
import { cva } from 'class-variance-authority';
import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from 'lucide-react';
import { cn } from '@design-system/utils';
import { Button } from '@design-system/components/primitives/Button';

const alertVariants = cva('relative flex items-start gap-3 rounded-xl border p-3 text-sm', {
  variants: {
    variant: {
      info: 'border-info-foreground/20 bg-info text-info-foreground',
      success: 'border-success-foreground/20 bg-success text-success-foreground',
      warning: 'border-warning-foreground/20 bg-warning text-warning-foreground',
      error: 'border-status-danger-foreground/20 bg-status-danger text-status-danger-foreground',
    },
    banner: { true: 'w-full rounded-none border-x-0', false: '' },
    inline: { true: 'items-center border-0 bg-transparent p-0', false: '' },
  },
  defaultVariants: { variant: 'info', banner: false, inline: false },
});
const icons = { info: Info, success: CircleCheck, warning: TriangleAlert, error: CircleAlert };

type Dismissal = { dismissible: true; dismissLabel: string; onDismiss?: () => void } | { dismissible?: false; dismissLabel?: never; onDismiss?: never };
export type AlertProps = Omit<ComponentProps<'div'>, 'title'> & Dismissal & {
  variant?: 'info' | 'success' | 'warning' | 'error';
  icon?: ReactNode;
  title?: ReactNode;
  action?: { label: string; onClick: () => void };
  banner?: boolean;
  inline?: boolean;
  hideIcon?: boolean;
};

export function Alert({ variant = 'info', icon, title, dismissible, dismissLabel, onDismiss, action, banner, inline = false, hideIcon, className, children, ...props }: AlertProps) {
  const [dismissed, setDismissed] = useState(false);
  if (dismissed) return null;
  const Icon = icons[variant];
  return (
    <div role={variant === 'error' ? 'alert' : 'status'} aria-live={variant === 'error' ? 'assertive' : 'polite'} aria-atomic="true" {...props} data-slot="alert" className={cn(alertVariants({ variant, banner, inline }), className)}>
      {!hideIcon && (icon ? <span aria-hidden="true" className={cn('shrink-0 [&_svg]:size-4 [&_svg]:stroke-[1.75]', !inline && 'mt-0.5')}>{icon}</span> : <Icon aria-hidden="true" className={cn('size-4 shrink-0 stroke-[1.75]', !inline && 'mt-0.5')} />)}
      <div className={cn('min-w-0 flex-1', !inline && 'space-y-2')}>
        {title && <AlertTitle>{title}</AlertTitle>}
        {children && <AlertDescription>{children}</AlertDescription>}
        {action && <Button variant="outline" size="sm" onClick={action.onClick}>{action.label}</Button>}
      </div>
      {dismissible && <Button variant="ghost" size="icon" aria-label={dismissLabel} className="shrink-0" onClick={() => { setDismissed(true); onDismiss?.(); }}><X aria-hidden="true" className="size-4" /></Button>}
    </div>
  );
}

export function AlertTitle({ className, children, ...props }: ComponentProps<'h3'>) {
  return <h3 {...props} data-slot="alert-title" className={cn('font-display font-semibold leading-tight text-inherit', className)}>{children}</h3>;
}

export function AlertDescription({ className, ...props }: ComponentProps<'div'>) {
  return <div {...props} data-slot="alert-description" className={cn('text-sm leading-relaxed text-inherit', className)} />;
}

export default Alert;
