/**
 * Popover Component
 *
 * Rich overlay component for displaying interactive content panels.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Built on Headless UI Popover for full accessibility
 * - 4 position variants: top, right, bottom, left
 * - 4 width variants: sm (300px), md (400px), lg (500px), full (90vw)
 * - Optional header section for titles
 * - Optional footer section for actions
 * - Optional arrow indicator pointing to trigger
 * - Controlled or uncontrolled mode
 * - Click outside to close behavior
 * - Keyboard focus support (Tab, Escape)
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React, { Fragment, useRef } from 'react';
import { Popover as HeadlessPopover, Transition } from '@headlessui/react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const popoverPanelVariants = cva(
  // Base styles - applied to all popovers
  'bg-white rounded-xl shadow-float border border-line-strong focus:outline-none',
  {
    variants: {
      width: {
        sm: 'w-[300px]',
        md: 'w-[400px]',
        lg: 'w-[500px]',
        full: 'w-[90vw]',
      },
    },
    defaultVariants: {
      width: 'md',
    },
  },
);

const positionVariants = cva('absolute z-50', {
  variants: {
    position: {
      top: 'bottom-full mb-2 left-1/2 -translate-x-1/2',
      right: 'left-full ml-2 top-1/2 -translate-y-1/2',
      bottom: 'top-full mt-2 left-1/2 -translate-x-1/2',
      left: 'right-full mr-2 top-1/2 -translate-y-1/2',
    },
  },
  defaultVariants: {
    position: 'bottom',
  },
});

const arrowVariants = cva(
  'absolute w-3 h-3 bg-white border-neutral-200 rotate-45',
  {
    variants: {
      position: {
        top: 'bottom-[-6px] left-1/2 -translate-x-1/2 border-b border-r',
        right: 'left-[-6px] top-1/2 -translate-y-1/2 border-l border-b',
        bottom: 'top-[-6px] left-1/2 -translate-x-1/2 border-t border-l',
        left: 'right-[-6px] top-1/2 -translate-y-1/2 border-t border-r',
      },
    },
    defaultVariants: {
      position: 'bottom',
    },
  },
);

export interface PopoverProps {
  /** Element that triggers the popover */
  trigger: React.ReactNode;
  /** Popover content */
  children: React.ReactNode;
  /** Optional header section (title or custom content) */
  header?: React.ReactNode;
  /** Optional footer section (actions or custom content) */
  footer?: React.ReactNode;
  /** Position of popover relative to trigger */
  position?: 'top' | 'right' | 'bottom' | 'left';
  /** Width variant */
  width?: 'sm' | 'md' | 'lg' | 'full';
  /** Whether to show arrow indicator */
  showArrow?: boolean;
  /** Controlled open state (optional) */
  isOpen?: boolean;
  /** Callback when open state changes (optional) */
  onOpenChange?: (open: boolean) => void;
  /** Additional CSS classes for the panel */
  className?: string;
  /** Additional CSS classes for the trigger wrapper */
  triggerClassName?: string;
}

/**
 * Popover component for displaying rich interactive content panels.
 * Uses Headless UI Popover for full accessibility and positioning.
 *
 * @example
 * ```tsx
 * <Popover trigger={<Button>Open</Button>}>
 *   <div>Popover content here</div>
 * </Popover>
 *
 * <Popover
 *   trigger={<IconButton icon={<InfoIcon />} />}
 *   header="User Information"
 *   position="right"
 *   showArrow
 * >
 *   <p>Detailed user information goes here.</p>
 * </Popover>
 *
 * <Popover
 *   trigger={<Button>Actions</Button>}
 *   header="Quick Actions"
 *   footer={
 *     <div className="flex gap-2 justify-end">
 *       <Button size="sm">Cancel</Button>
 *       <Button size="sm" variant="primary">Apply</Button>
 *     </div>
 *   }
 * >
 *   <div>Select an action to perform</div>
 * </Popover>
 * ```
 */
export const Popover = React.forwardRef<HTMLDivElement, PopoverProps>(
  (
    {
      trigger,
      children,
      header,
      footer,
      position = 'bottom',
      width = 'md',
      showArrow = true,
      isOpen,
      onOpenChange,
      className,
      triggerClassName,
      ...props
    },
    ref,
  ) => {
    // If controlled mode, use HeadlessPopover.Group pattern
    // Otherwise, use uncontrolled mode
    const isControlled = isOpen !== undefined && onOpenChange !== undefined;
    const lastOpenRef = useRef<boolean | null>(null);

    const PopoverContent = (
      <>
        <HeadlessPopover.Button
          className={cn(
            'inline-flex items-center focus:outline-none focus:ring-2 focus:ring-trust-deep focus:ring-offset-2 rounded-md',
            triggerClassName,
          )}
        >
          {trigger}
        </HeadlessPopover.Button>

        <Transition
          as={Fragment}
          enter="transition ease-out duration-200"
          enterFrom="opacity-0 scale-95"
          enterTo="opacity-100 scale-100"
          leave="transition ease-in duration-150"
          leaveFrom="opacity-100 scale-100"
          leaveTo="opacity-0 scale-95"
        >
          <HeadlessPopover.Panel
            className={cn(positionVariants({ position }))}
            ref={ref}
          >
            <div
              className={cn(popoverPanelVariants({ width }), className)}
              {...props}
            >
              {/* Arrow indicator */}
              {showArrow && <div className={cn(arrowVariants({ position }))} />}

              {/* Header */}
              {header && (
                <div className="px-5 py-4 border-b border-neutral-200">
                  {typeof header === 'string' ? (
                    <h3 className="text-base font-medium text-neutral-900">
                      {header}
                    </h3>
                  ) : (
                    header
                  )}
                </div>
              )}

              {/* Content */}
              <div
                className={cn(
                  'px-5 py-4 text-sm text-neutral-700',
                  !header && 'pt-5',
                  !footer && 'pb-5',
                )}
              >
                {children}
              </div>

              {/* Footer */}
              {footer && (
                <div className="px-5 py-3 border-t border-neutral-200 bg-neutral-50 rounded-b-lg">
                  {footer}
                </div>
              )}
            </div>
          </HeadlessPopover.Panel>
        </Transition>
      </>
    );

    // Controlled mode
    if (isControlled) {
      return (
        <HeadlessPopover className="relative inline-flex">
          {({ open }) => {
            // Sync controlled state with Headless UI internal state
            if (open !== lastOpenRef.current) {
              lastOpenRef.current = open;
              if (open !== isOpen) {
                onOpenChange(open);
              }
            }

            return PopoverContent;
          }}
        </HeadlessPopover>
      );
    }

    // Uncontrolled mode
    return (
      <HeadlessPopover className="relative inline-flex">
        {PopoverContent}
      </HeadlessPopover>
    );
  },
);

Popover.displayName = 'Popover';

export default Popover;
