/**
 * Modal Component
 *
 * Accessible dialog overlay for forms, confirmations, and content.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Built on Headless UI Dialog for full accessibility
 * - 3 sizes: sm (400px), md (600px), lg (800px)
 * - Optional scrollable content area
 * - Optional header icon
 * - Configurable backdrop click behavior
 * - Smooth fade-in/slide-down animations
 * - Focus management and keyboard interactions (ESC to close)
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React from 'react';
import { Dialog, Transition } from '@headlessui/react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { X } from 'lucide-react';

const modalVariants = cva(
  // Base styles - applied to all modals
  'relative bg-white rounded-2xl border border-line-strong shadow-float transition-[transform,opacity]',
  {
    variants: {
      size: {
        sm: 'w-full max-w-md',
        md: 'w-full max-w-2xl',
        lg: 'w-full max-w-4xl',
      },
      scrollable: {
        true: 'flex flex-col max-h-[85vh]',
        false: '',
      },
    },
    defaultVariants: {
      size: 'md',
      scrollable: false,
    },
  },
);

const CloseIcon = () => (
  <X className="w-5 h-5" aria-hidden="true" strokeWidth={1.5} />
);

export interface ModalProps {
  /** Whether the modal is open */
  isOpen: boolean;
  /** Callback when modal should close */
  onClose: () => void;
  /** Optional modal title */
  title?: string;
  /** Modal content */
  children: React.ReactNode;
  /** Optional footer content (typically buttons) */
  footer?: React.ReactNode;
  /** Modal size */
  size?: 'sm' | 'md' | 'lg';
  /** Enable scrollable content area */
  scrollable?: boolean;
  /** Whether clicking backdrop closes the modal */
  closeOnBackdropClick?: boolean;
  /** Optional icon to display in header */
  icon?: React.ReactNode;
  /** Additional class names for the modal panel */
  className?: string;
}

/**
 * Modal component for displaying overlay dialogs.
 * Uses Headless UI Dialog for full accessibility and focus management.
 *
 * @example
 * ```tsx
 * <Modal
 *   isOpen={isOpen}
 *   onClose={() => setIsOpen(false)}
 *   title="Confirm Action"
 * >
 *   Are you sure you want to proceed?
 * </Modal>
 *
 * <Modal
 *   isOpen={isOpen}
 *   onClose={() => setIsOpen(false)}
 *   title="User Profile"
 *   size="lg"
 *   footer={
 *     <div className="flex gap-3 justify-end">
 *       <Button variant="outline" onClick={() => setIsOpen(false)}>
 *         Cancel
 *       </Button>
 *       <Button variant="primary" onClick={handleSave}>
 *         Save Changes
 *       </Button>
 *     </div>
 *   }
 * >
 *   <form>...</form>
 * </Modal>
 * ```
 */
export const Modal = React.forwardRef<HTMLDivElement, ModalProps>(
  (
    {
      isOpen,
      onClose,
      title,
      children,
      footer,
      size = 'md',
      scrollable = false,
      closeOnBackdropClick = true,
      icon,
      className,
    },
    ref,
  ) => {
    const handleBackdropClick = () => {
      if (closeOnBackdropClick) {
        onClose();
      }
    };

    return (
      <Transition show={isOpen} as={React.Fragment}>
        <Dialog
          as="div"
          className="relative z-50"
          onClose={handleBackdropClick}
          initialFocus={undefined}
        >
          {/* Backdrop */}
          <Transition.Child
            as={React.Fragment}
            enter="ease-out duration-300"
            enterFrom="opacity-0"
            enterTo="opacity-100"
            leave="ease-in duration-200"
            leaveFrom="opacity-100"
            leaveTo="opacity-0"
          >
            <div className="fixed inset-0 bg-[rgba(13,24,41,0.5)]" />
          </Transition.Child>

          {/* Full-screen container */}
          <div className="fixed inset-0 overflow-y-auto">
            <div className="flex min-h-full items-center justify-center p-4">
              {/* Modal panel */}
              <Transition.Child
                as={React.Fragment}
                enter="ease-out duration-300"
                enterFrom="opacity-0 scale-95 translate-y-4"
                enterTo="opacity-100 scale-100 translate-y-0"
                leave="ease-in duration-200"
                leaveFrom="opacity-100 scale-100 translate-y-0"
                leaveTo="opacity-0 scale-95 translate-y-4"
              >
                <Dialog.Panel
                  ref={ref}
                  className={cn(modalVariants({ size, scrollable }), className)}
                >
                  {/* Header */}
                  {(title || icon) && (
                    <div
                      className={cn(
                        'px-6 py-5 border-b border-neutral-200',
                        scrollable && 'flex-shrink-0',
                      )}
                    >
                      <div className="flex items-start justify-between gap-4">
                        <div className="flex items-start gap-3 flex-1 min-w-0">
                          {icon && (
                            <div className="flex-shrink-0 w-6 h-6 text-trust-deep mt-0.5">
                              {icon}
                            </div>
                          )}
                          {title && (
                            <Dialog.Title
                              as="h3"
                              className="text-lg font-medium text-neutral-900 leading-6"
                            >
                              {title}
                            </Dialog.Title>
                          )}
                        </div>
                        <button
                          type="button"
                          onClick={onClose}
                          aria-label="Close modal"
                          className="flex-shrink-0 inline-flex items-center justify-center w-8 h-8 text-neutral-400 hover:text-neutral-500 hover:bg-neutral-100 rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-trust-deep focus:ring-offset-2"
                        >
                          <CloseIcon />
                        </button>
                      </div>
                    </div>
                  )}

                  {/* Content */}
                  <div
                    className={cn(
                      'px-6 py-5',
                      scrollable
                        ? 'overflow-y-auto flex-1'
                        : 'overflow-visible',
                      !title && !icon && 'pt-6',
                    )}
                  >
                    {children}
                  </div>

                  {/* Footer */}
                  {footer && (
                    <div
                      className={cn(
                        'px-6 py-4 border-t border-neutral-200 bg-neutral-50 rounded-b-2xl',
                        scrollable && 'flex-shrink-0',
                      )}
                    >
                      {footer}
                    </div>
                  )}
                </Dialog.Panel>
              </Transition.Child>
            </div>
          </div>
        </Dialog>
      </Transition>
    );
  },
);

Modal.displayName = 'Modal';

export default Modal;
