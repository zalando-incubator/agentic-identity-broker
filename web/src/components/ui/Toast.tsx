/**
 * Toast notification component with auto-dismiss and animations.
 *
 * Features:
 * - Auto-dismiss after configurable duration (default 4s)
 * - Manual dismiss with close button
 * - Multiple variants: success, error, warning, info
 * - Framer Motion animations (fade + slide)
 * - Positioned at top-right corner
 * - Accessible with ARIA attributes
 *
 * Usage:
 * ```tsx
 * import { Toast, ToastProvider, useToast } from '@components/ui/Toast';
 *
 * function MyComponent() {
 *   const { showToast } = useToast();
 *   return (
 *     <button onClick={() => showToast('Success!', 'success')}>
 *       Show Toast
 *     </button>
 *   );
 * }
 * ```
 */

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
} from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { CircleCheck, CircleX, Info, TriangleAlert, X } from 'lucide-react';

export type ToastVariant = 'success' | 'error' | 'warning' | 'info';

export interface ToastMessage {
  id: string;
  message: string;
  variant: ToastVariant;
  duration?: number;
}

interface ToastContextValue {
  showToast: (
    message: string,
    variant?: ToastVariant,
    duration?: number,
  ) => void;
  hideToast: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | undefined>(undefined);

/**
 * Hook to access toast notifications.
 * Must be used within a ToastProvider.
 */
export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return context;
}

interface ToastProviderProps {
  children: React.ReactNode;
  maxToasts?: number;
}

/**
 * ToastProvider manages toast notification state and provides context.
 * Wrap your app or component tree with this provider.
 */
export function ToastProvider({ children, maxToasts = 5 }: ToastProviderProps) {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  /**
   * Show a new toast notification.
   */
  const showToast = useCallback(
    (
      message: string,
      variant: ToastVariant = 'info',
      duration: number = 4000,
    ) => {
      const id = `toast-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
      const newToast: ToastMessage = { id, message, variant, duration };

      setToasts((prev) => {
        // Limit number of toasts
        const updated = [...prev, newToast];
        if (updated.length > maxToasts) {
          return updated.slice(updated.length - maxToasts);
        }
        return updated;
      });
    },
    [maxToasts],
  );

  /**
   * Hide a specific toast by ID.
   */
  const hideToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ showToast, hideToast }}>
      {children}
      <ToastContainer toasts={toasts} onDismiss={hideToast} />
    </ToastContext.Provider>
  );
}

interface ToastContainerProps {
  toasts: ToastMessage[];
  onDismiss: (id: string) => void;
}

/**
 * ToastContainer renders all active toasts in a fixed position.
 */
function ToastContainer({ toasts, onDismiss }: ToastContainerProps) {
  return (
    <div
      className="fixed top-4 right-4 z-50 flex flex-col gap-2 pointer-events-none"
      role="region"
      aria-live="polite"
      aria-label="Notifications"
    >
      <AnimatePresence>
        {toasts.map((toast) => (
          <Toast key={toast.id} toast={toast} onDismiss={onDismiss} />
        ))}
      </AnimatePresence>
    </div>
  );
}

interface ToastProps {
  toast: ToastMessage;
  onDismiss: (id: string) => void;
}

/**
 * Individual Toast component with auto-dismiss timer.
 */
function Toast({ toast, onDismiss }: ToastProps) {
  const { id, message, variant, duration = 4000 } = toast;

  // Auto-dismiss after duration
  useEffect(() => {
    if (duration > 0) {
      const timer = setTimeout(() => {
        onDismiss(id);
      }, duration);

      return () => clearTimeout(timer);
    }
  }, [id, duration, onDismiss]);

  // Variant-specific styles
  const variantStyles = {
    success: {
      bg: 'bg-success-light',
      border: 'border-success-primary/30',
      icon: 'text-success-primary',
      text: 'text-success-dark',
    },
    error: {
      bg: 'bg-error-light',
      border: 'border-error-primary/30',
      icon: 'text-error-primary',
      text: 'text-error-dark',
    },
    warning: {
      bg: 'bg-warning-light',
      border: 'border-warning-primary/30',
      icon: 'text-warning-primary',
      text: 'text-warning-dark',
    },
    info: {
      bg: 'bg-info-light',
      border: 'border-info-primary/30',
      icon: 'text-info-primary',
      text: 'text-info-dark',
    },
  };

  const styles = variantStyles[variant];

  return (
    <motion.div
      initial={{ opacity: 0, x: 100, scale: 0.95 }}
      animate={{ opacity: 1, x: 0, scale: 1 }}
      exit={{ opacity: 0, x: 100, scale: 0.95 }}
      transition={{ duration: 0.2 }}
      className={`
        ${styles.bg} ${styles.border} ${styles.text}
        border rounded-lg shadow-float p-4 pr-10
        min-w-[320px] max-w-md
        pointer-events-auto
        relative
      `}
      role={variant === 'error' || variant === 'warning' ? 'alert' : 'status'}
    >
      <div className="flex items-start gap-3">
        {/* Icon */}
        <div className={`${styles.icon} flex-shrink-0`}>
          {variant === 'success' && <SuccessIcon />}
          {variant === 'error' && <ErrorIcon />}
          {variant === 'warning' && <WarningIcon />}
          {variant === 'info' && <InfoIcon />}
        </div>

        {/* Message */}
        <p className="text-sm font-medium flex-1 pt-0.5">{message}</p>

        {/* Close button */}
        <button
          onClick={() => onDismiss(id)}
          className={`
            ${styles.icon} hover:opacity-70
            absolute top-3 right-3
            transition-opacity
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-accent
            rounded
          `}
          aria-label="Close notification"
        >
          <X className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
        </button>
      </div>
    </motion.div>
  );
}

// Icon components
function SuccessIcon() {
  return (
    <CircleCheck className="w-6 h-6" aria-hidden="true" strokeWidth={1.5} />
  );
}

function ErrorIcon() {
  return <CircleX className="w-6 h-6" aria-hidden="true" strokeWidth={1.5} />;
}

function WarningIcon() {
  return (
    <TriangleAlert className="w-6 h-6" aria-hidden="true" strokeWidth={1.5} />
  );
}

function InfoIcon() {
  return <Info className="w-6 h-6" aria-hidden="true" strokeWidth={1.5} />;
}

export default Toast;
