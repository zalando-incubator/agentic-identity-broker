/**
 * Toast Component
 *
 * Temporary notification component for non-blocking feedback messages.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 4 semantic variants: info, success, warning, error
 * - Optional icon support
 * - Optional title with description
 * - Auto-dismiss with configurable duration
 * - Manual dismiss button
 * - Optional action button
 * - Positioning: top-right, top-left, bottom-right, bottom-left
 * - Slide-in animation with fade-out on dismiss
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React, { useState, useEffect, useCallback } from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { CircleCheck, CircleX, Info, TriangleAlert, X } from 'lucide-react';

const toastVariants = cva(
  // Base styles - applied to all variants
  'relative flex gap-3 rounded-lg border p-4 shadow-float max-w-md w-full transition-[transform,opacity] duration-200',
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
      position: {
        'top-right': 'fixed top-4 right-4',
        'top-left': 'fixed top-4 left-4',
        'bottom-right': 'fixed bottom-4 right-4',
        'bottom-left': 'fixed bottom-4 left-4',
      },
    },
    defaultVariants: {
      variant: 'info',
      position: 'top-right',
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

export interface ToastProps extends Omit<
  React.ComponentPropsWithoutRef<'div'>,
  'title'
> {
  /** Toast variant based on message severity */
  variant?: 'info' | 'success' | 'warning' | 'error';
  /** Optional icon to display (overrides default icon) */
  icon?: React.ReactNode;
  /** Optional title text (bold) */
  title?: string;
  /** Auto-dismiss duration in milliseconds (0 = no auto-dismiss) */
  duration?: number;
  /** Callback when toast is dismissed */
  onDismiss?: () => void;
  /** Optional action button */
  action?: {
    label: string;
    onClick: () => void;
  };
  /** Toast position on screen */
  position?: 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left';
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
 * Toast component for displaying temporary notification messages.
 * Supports multiple variants, icons, titles, actions, and auto-dismiss.
 *
 * @example
 * ```tsx
 * <Toast variant="success" duration={5000}>
 *   Your changes have been saved successfully.
 * </Toast>
 *
 * <Toast variant="warning" title="Warning" position="top-right">
 *   Please review the following information before proceeding.
 * </Toast>
 *
 * <Toast
 *   variant="error"
 *   title="Error"
 *   action={{ label: "Retry", onClick: handleRetry }}
 *   duration={0}
 * >
 *   Failed to process your request.
 * </Toast>
 * ```
 */
export const Toast = React.forwardRef<HTMLDivElement, ToastProps>(
  (
    {
      variant = 'info',
      icon,
      title,
      duration = 5000,
      onDismiss,
      action,
      position = 'top-right',
      hideIcon = false,
      className,
      children,
      ...props
    },
    ref,
  ) => {
    const [isVisible, setIsVisible] = useState(true);
    const [isExiting, setIsExiting] = useState(false);
    const [isEntering, setIsEntering] = useState(true);

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
    const handleDismiss = useCallback(() => {
      setIsExiting(true);
      setTimeout(() => {
        setIsVisible(false);
        onDismiss?.();
      }, 200); // Match transition duration
    }, [onDismiss]);

    // Auto-dismiss after duration
    useEffect(() => {
      if (duration > 0) {
        const timer = setTimeout(() => {
          handleDismiss();
        }, duration);

        return () => clearTimeout(timer);
      }
    }, [duration, handleDismiss]);

    // Entrance animation
    useEffect(() => {
      // Remove entering state after animation completes
      const timer = setTimeout(() => {
        setIsEntering(false);
      }, 200);

      return () => clearTimeout(timer);
    }, []);

    // Don't render if dismissed
    if (!isVisible) {
      return null;
    }

    // Determine ARIA role and aria-live based on variant
    const role = variant === 'error' ? 'alert' : 'status';
    const ariaLive = variant === 'error' ? 'assertive' : 'polite';

    // Determine slide direction based on position
    const getSlideAnimation = () => {
      if (isExiting) {
        return 'opacity-0 scale-95';
      }
      if (isEntering) {
        if (position.includes('right')) {
          return 'translate-x-full opacity-0';
        }
        if (position.includes('left')) {
          return '-translate-x-full opacity-0';
        }
      }
      return '';
    };

    return (
      <div
        ref={ref}
        role={role}
        aria-live={ariaLive}
        className={cn(
          toastVariants({ variant, position }),
          getSlideAnimation(),
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

        {/* Dismiss button - always shown for manual control */}
        <button
          type="button"
          onClick={handleDismiss}
          aria-label="Dismiss notification"
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
      </div>
    );
  },
);

Toast.displayName = 'Toast';

export default Toast;
