import { CircleCheck, CircleX, Info, LoaderCircle, TriangleAlert, X } from 'lucide-react';
import { Toaster as Sonner, type ToasterProps as SonnerProps } from 'sonner';
import { useTheme } from '@design-system/theme/ThemeProvider';
import { cn } from '@design-system/utils';
import './Toaster.css';

export interface ToasterProps extends Omit<SonnerProps, 'theme' | 'richColors' | 'invert' | 'customAriaLabel' | 'containerAriaLabel' | 'toastOptions' | 'icons'> {
  label: string;
  closeButtonLabel: string;
}

export function Toaster({ label, closeButtonLabel, className, closeButton = true, ...props }: ToasterProps) {
  const { resolvedTheme } = useTheme();
  return <Sonner {...props} theme={resolvedTheme} className={cn('aib-toaster', className)} closeButton={closeButton} customAriaLabel={label} icons={{
    success: <CircleCheck aria-hidden="true" className="size-5 text-success" />,
    info: <Info aria-hidden="true" className="size-5 text-info" />,
    warning: <TriangleAlert aria-hidden="true" className="size-5 text-warning" />,
    error: <CircleX aria-hidden="true" className="size-5 text-destructive" />,
    loading: <LoaderCircle aria-hidden="true" className="size-5 text-muted-foreground motion-safe:animate-spin" />,
    close: <X aria-hidden="true" className="size-4" />,
  }} toastOptions={{
    unstyled: true,
    closeButtonAriaLabel: closeButtonLabel,
    classNames: {
      toast: 'flex w-full items-center gap-3 rounded-lg border border-border bg-popover p-4 pr-12 text-popover-foreground',
      title: 'text-sm font-medium',
      description: 'text-sm',
      content: 'min-w-0 flex-1 space-y-1',
      icon: 'shrink-0',
      closeButton: 'absolute right-2 top-2 inline-flex size-8 items-center justify-center rounded-md border border-border',
      actionButton: 'rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-secondary-foreground',
      cancelButton: 'rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-secondary-foreground',
    },
  }} />;
}
