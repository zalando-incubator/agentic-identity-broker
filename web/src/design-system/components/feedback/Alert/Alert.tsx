/**
 * Alert Component
 *
 * Contextual feedback component for displaying important messages.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 4 semantic variants: info, success, warning, error
 * - Optional icon support
 * - Optional title with description
 * - Dismissible with fade-out animation
 * - Optional action button
 * - Banner style for full-width alerts
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React, { useState } from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { CircleCheck, CircleX, Info, TriangleAlert, X } from 'lucide-react';

const alertVariants = cva(
  // Base styles - applied to all variants
  'relative flex gap-3 rounded-lg border p-4 transition-opacity duration-200',
  {
    variants: {
      variant: {
        // Info: Blue - informational messages
        info: 'bg-info-light text-info-dark border-info-primary/30',

        // Success: Green - positive confirmations
        success: 'bg-success-light text-success-dark border-success-primary/30',

        // Warning: Amber - caution messages
        warning: 'bg-warning-light text-warning-dark border-warning-primary/30',

        // Error: Red - error messages
        error: 'bg-error-light text-error-dark border-error-primary/30',
      },
      banner: {
        true: 'rounded-none border-l-0 border-r-0 w-full',
        false: '',
      },
    },
    defaultVariants: {
      variant: 'info',
      banner: false,
    },
  },
);

const iconColorVariants = cva('flex-shrink-0 w-5 h-5', {
  variants: {
    variant: {
      info: 'text-info-primary',
      success: 'text-success-primary',
      warning: 'text-warning-primary',
      error: 'text-error-primary',
    },
  },
  defaultVariants: {
    variant: 'info',
  },
});

export interface AlertProps extends Omit<
  React.ComponentPropsWithoutRef<'div'>,
  'title'
> {
  /** Alert variant based on message severity */
  variant?: 'info' | 'success' | 'warning' | 'error';
  /** Optional icon to display (overrides default icon) */
  icon?: React.ReactNode;
  /** Optional title text (bold) */
  title?: string;
  /** Whether alert can be dismissed */
  dismissible?: boolean;
  /** Callback when alert is dismissed */
  onDismiss?: () => void;
  /** Optional action button */
  action?: {
    label: string;
    onClick: () => void;
  };
  /** Display as full-width banner */
  banner?: boolean;
  /** Hide default icon */
  hideIcon?: boolean;
}

// Default icons for each variant (SVG)
const InfoIcon = () => (
  <Info className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const SuccessIcon = () => (
  <CircleCheck className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const WarningIcon = () => (
  <TriangleAlert className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const ErrorIcon = () => (
  <CircleX className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

const CloseIcon = () => (
  <X className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

/**
 * Alert component for displaying contextual feedback messages.
 * Supports multiple variants, icons, titles, and actions.
 *
 * @example
 * ```tsx
 * <Alert variant="success">
 *   Your changes have been saved successfully.
 * </Alert>
 *
 * <Alert variant="warning" title="Warning" dismissible>
 *   Please review the following information before proceeding.
 * </Alert>
 *
 * <Alert
 *   variant="error"
 *   title="Error"
 *   action={{ label: "Retry", onClick: handleRetry }}
 * >
 *   Failed to process your request.
 * </Alert>
 * ```
 */
export const Alert = React.forwardRef<HTMLDivElement, AlertProps>(
  (
    {
      variant = 'info',
      icon,
      title,
      dismissible = false,
      onDismiss,
      action,
      banner = false,
      hideIcon = false,
      className,
      children,
      ...props
    },
    ref,
  ) => {
    const [isVisible, setIsVisible] = useState(true);
    const [isExiting, setIsExiting] = useState(false);

    // Get default icon based on variant
    const getDefaultIcon = () => {
      switch (variant) {
        case 'success':
          return <SuccessIcon />;
        case 'warning':
          return <WarningIcon />;
        case 'error':
          return <ErrorIcon />;
        case 'info':
        default:
          return <InfoIcon />;
      }
    };

    // Handle dismiss with animation
    const handleDismiss = () => {
      setIsExiting(true);
      setTimeout(() => {
        setIsVisible(false);
        onDismiss?.();
      }, 300); // Match transition duration
    };

    // Don't render if dismissed
    if (!isVisible) {
      return null;
    }

    // Determine ARIA role and aria-live based on variant
    const role = variant === 'error' ? 'alert' : 'status';
    const ariaLive = variant === 'error' ? 'assertive' : 'polite';

    return (
      <div
        ref={ref}
        role={role}
        aria-live={ariaLive}
        className={cn(
          alertVariants({ variant, banner }),
          isExiting && 'opacity-0 scale-95',
          className,
        )}
        {...props}
      >
        {/* Icon */}
        {!hideIcon && (
          <div className={iconColorVariants({ variant })}>
            {icon || getDefaultIcon()}
          </div>
        )}

        {/* Content */}
        <div className="flex-1 min-w-0">
          {title && (
            <h4 className="text-sm font-medium mb-1 leading-tight">{title}</h4>
          )}
          <div className="text-sm leading-relaxed">{children}</div>
        </div>

        {/* Action button */}
        {action && (
          <button
            type="button"
            onClick={action.onClick}
            className={cn(
              'flex-shrink-0 px-3 py-1.5 text-sm font-medium rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2',
              variant === 'info' &&
                'text-info-primary hover:bg-info-primary/10 focus:ring-info-primary',
              variant === 'success' &&
                'text-success-primary hover:bg-success-primary/10 focus:ring-success-primary',
              variant === 'warning' &&
                'text-warning-primary hover:bg-warning-primary/10 focus:ring-warning-primary',
              variant === 'error' &&
                'text-error-primary hover:bg-error-primary/10 focus:ring-error-primary',
            )}
          >
            {action.label}
          </button>
        )}

        {/* Dismiss button */}
        {dismissible && (
          <button
            type="button"
            onClick={handleDismiss}
            aria-label="Dismiss alert"
            className={cn(
              'flex-shrink-0 inline-flex items-center justify-center w-8 h-8 rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2',
              variant === 'info' &&
                'text-info-primary/70 hover:text-info-primary hover:bg-info-primary/10 focus:ring-info-primary',
              variant === 'success' &&
                'text-success-primary/70 hover:text-success-primary hover:bg-success-primary/10 focus:ring-success-primary',
              variant === 'warning' &&
                'text-warning-primary/70 hover:text-warning-primary hover:bg-warning-primary/10 focus:ring-warning-primary',
              variant === 'error' &&
                'text-error-primary/70 hover:text-error-primary hover:bg-error-primary/10 focus:ring-error-primary',
            )}
          >
            <CloseIcon />
          </button>
        )}
      </div>
    );
  },
);

Alert.displayName = 'Alert';

export default Alert;
