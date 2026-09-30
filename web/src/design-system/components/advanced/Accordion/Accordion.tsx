/**
 * Accordion Component
 *
 * Collapsible panel component for organizing content sections.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Multiple collapsible panels
 * - Single or multiple open panels (exclusive vs non-exclusive)
 * - Header with title, optional icon, chevron indicator
 * - Smooth expand/collapse animations
 * - 2 size variants (sm, md)
 * - Optional descriptions/subtitles in header
 * - Keyboard navigation (ArrowUp/Down, Enter/Space)
 * - Support for nested accordions
 * - Optional header actions (badges, icons)
 * - WCAG 2.1 AA accessible with proper ARIA
 */

import React, { useState, useEffect } from 'react';
import { Transition } from '@headlessui/react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const accordionVariants = cva(
  // Base accordion container styles
  'divide-y divide-neutral-200 border border-neutral-200 rounded-lg overflow-hidden',
  {
    variants: {
      size: {
        sm: '',
        md: '',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
);

const accordionItemVariants = cva(
  // Base item button styles
  'w-full flex items-center justify-between gap-3 text-left transition-colors duration-150',
  {
    variants: {
      size: {
        sm: 'px-4 py-3 text-sm',
        md: 'px-6 py-4 text-base',
      },
      disabled: {
        true: 'cursor-not-allowed opacity-50',
        false: 'hover:bg-neutral-50 cursor-pointer',
      },
      open: {
        true: 'bg-neutral-50',
        false: 'bg-white',
      },
    },
    defaultVariants: {
      size: 'md',
      disabled: false,
      open: false,
    },
  },
);

const accordionContentVariants = cva(
  // Base content area styles
  'border-t border-neutral-200 bg-white',
  {
    variants: {
      size: {
        sm: 'px-4 py-3 text-sm',
        md: 'px-6 py-4 text-base',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
);

const ChevronDownIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg
    className={cn('w-5 h-5 transition-transform duration-200', className)}
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    aria-hidden="true"
  >
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M19 9l-7 7-7-7"
    />
  </svg>
);

export interface AccordionItem {
  /** Unique identifier for the item */
  id: string;
  /** Item title */
  title: string;
  /** Optional description/subtitle */
  description?: string;
  /** Content to display when expanded */
  content: React.ReactNode;
  /** Optional icon to display before title */
  icon?: React.ReactNode;
  /** Whether the item is disabled */
  disabled?: boolean;
  /** Optional badge/action element */
  badge?: React.ReactNode;
}

export interface AccordionProps {
  /** Array of accordion items */
  items: AccordionItem[];
  /** Size variant */
  size?: 'sm' | 'md';
  /** Only allow one item open at a time */
  exclusive?: boolean;
  /** ID(s) of initially open items */
  defaultOpen?: string | string[];
  /** Callback when open items change */
  onChange?: (openIds: string[]) => void;
  /** Additional class names */
  className?: string;
}

/**
 * Accordion component for collapsible content sections.
 *
 * @example
 * ```tsx
 * // Basic accordion
 * <Accordion
 *   items={[
 *     { id: '1', title: 'Section 1', content: 'Content 1' },
 *     { id: '2', title: 'Section 2', content: 'Content 2' },
 *   ]}
 * />
 *
 * // Non-exclusive with multiple open
 * <Accordion
 *   exclusive={false}
 *   defaultOpen={['1', '2']}
 *   items={items}
 * />
 *
 * // With icons and badges
 * <Accordion
 *   items={[
 *     {
 *       id: '1',
 *       title: 'Settings',
 *       icon: <SettingsIcon />,
 *       badge: <Badge>3</Badge>,
 *       content: <SettingsForm />
 *     }
 *   ]}
 * />
 * ```
 */
export const Accordion = React.forwardRef<HTMLDivElement, AccordionProps>(
  (
    { items, size = 'md', exclusive = true, defaultOpen, onChange, className },
    ref,
  ) => {
    // Track open items internally
    const [openItems, setOpenItems] = useState<Set<string>>(() => {
      if (!defaultOpen) return new Set();
      return new Set(Array.isArray(defaultOpen) ? defaultOpen : [defaultOpen]);
    });

    // Notify parent of changes
    useEffect(() => {
      if (onChange) {
        onChange(Array.from(openItems));
      }
    }, [openItems, onChange]);

    const handleToggle = (itemId: string) => {
      setOpenItems((prev) => {
        const next = new Set(prev);

        if (next.has(itemId)) {
          // Close the item
          next.delete(itemId);
        } else {
          // Open the item
          if (exclusive) {
            // In exclusive mode, close all others
            next.clear();
          }
          next.add(itemId);
        }

        return next;
      });
    };

    // Handle keyboard navigation
    const handleKeyDown = (
      event: React.KeyboardEvent,
      currentIndex: number,
    ) => {
      const enabledItems = items.filter((item) => !item.disabled);
      const currentEnabledIndex = enabledItems.findIndex(
        (item) => item === items[currentIndex],
      );

      if (event.key === 'ArrowDown') {
        event.preventDefault();
        const nextIndex = (currentEnabledIndex + 1) % enabledItems.length;
        const nextItem = enabledItems[nextIndex];
        const nextButton = document.querySelector(
          `[data-accordion-button="${nextItem.id}"]`,
        ) as HTMLButtonElement;
        nextButton?.focus();
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        const prevIndex =
          currentEnabledIndex === 0
            ? enabledItems.length - 1
            : currentEnabledIndex - 1;
        const prevItem = enabledItems[prevIndex];
        const prevButton = document.querySelector(
          `[data-accordion-button="${prevItem.id}"]`,
        ) as HTMLButtonElement;
        prevButton?.focus();
      } else if (event.key === 'Home') {
        event.preventDefault();
        const firstItem = enabledItems[0];
        const firstButton = document.querySelector(
          `[data-accordion-button="${firstItem.id}"]`,
        ) as HTMLButtonElement;
        firstButton?.focus();
      } else if (event.key === 'End') {
        event.preventDefault();
        const lastItem = enabledItems[enabledItems.length - 1];
        const lastButton = document.querySelector(
          `[data-accordion-button="${lastItem.id}"]`,
        ) as HTMLButtonElement;
        lastButton?.focus();
      }
    };

    if (items.length === 0) {
      return (
        <div
          ref={ref}
          className={cn(
            'border border-neutral-200 rounded-lg p-8 text-center text-neutral-500',
            className,
          )}
        >
          <p className="text-sm">No items to display</p>
        </div>
      );
    }

    return (
      <div ref={ref} className={cn(accordionVariants({ size }), className)}>
        {items.map((item, index) => {
          const isItemOpen = openItems.has(item.id);

          return (
            <div key={item.id}>
              <button
                type="button"
                disabled={item.disabled}
                onClick={() => handleToggle(item.id)}
                onKeyDown={(e) => handleKeyDown(e, index)}
                data-accordion-button={item.id}
                className={cn(
                  accordionItemVariants({
                    size,
                    disabled: item.disabled,
                    open: isItemOpen,
                  }),
                )}
                aria-expanded={isItemOpen}
                aria-controls={`accordion-content-${item.id}`}
              >
                <div className="flex items-start gap-3 flex-1 min-w-0">
                  {item.icon && (
                    <div className="flex-shrink-0 w-5 h-5 text-trust-deep mt-0.5">
                      {item.icon}
                    </div>
                  )}
                  <div className="flex-1 min-w-0">
                    <div className="font-semibold text-neutral-900">
                      {item.title}
                    </div>
                    {item.description && (
                      <div className="mt-1 text-sm text-neutral-600">
                        {item.description}
                      </div>
                    )}
                  </div>
                  {item.badge && (
                    <div className="flex-shrink-0">{item.badge}</div>
                  )}
                </div>
                <ChevronDownIcon
                  className={cn(
                    'flex-shrink-0 text-neutral-400',
                    isItemOpen && 'rotate-180',
                  )}
                />
              </button>

              <Transition
                show={isItemOpen}
                enter="transition duration-200 ease-out"
                enterFrom="transform scale-95 opacity-0"
                enterTo="transform scale-100 opacity-100"
                leave="transition duration-150 ease-out"
                leaveFrom="transform scale-100 opacity-100"
                leaveTo="transform scale-95 opacity-0"
              >
                <div
                  id={`accordion-content-${item.id}`}
                  className={accordionContentVariants({ size })}
                >
                  {item.content}
                </div>
              </Transition>
            </div>
          );
        })}
      </div>
    );
  },
);

Accordion.displayName = 'Accordion';

export default Accordion;
