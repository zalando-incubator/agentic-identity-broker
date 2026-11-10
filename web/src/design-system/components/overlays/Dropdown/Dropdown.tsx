/**
 * Dropdown Component
 *
 * Accessible dropdown menu for action lists and contextual options.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Built on Headless UI Menu for full accessibility
 * - 3 sizes: sm, md, lg
 * - Section grouping with dividers
 * - Icons, descriptions, and checkmarks
 * - Disabled and destructive item states
 * - Auto-positioning with align control
 * - Keyboard navigation (arrow keys, enter, ESC)
 * - WCAG 2.1 AA compliant with proper ARIA attributes
 */

import React from 'react';
import { Menu, Transition } from '@headlessui/react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { Check, ChevronDown } from 'lucide-react';

const dropdownVariants = cva(
  // Base styles - applied to all dropdowns
  'absolute z-50 mt-2 rounded-xl bg-white border border-line-strong shadow-float focus:outline-none overflow-hidden',
  {
    variants: {
      size: {
        sm: 'min-w-[160px]',
        md: 'min-w-[200px]',
        lg: 'min-w-[280px]',
      },
      align: {
        left: 'left-0 origin-top-left',
        right: 'right-0 origin-top-right',
      },
    },
    defaultVariants: {
      size: 'md',
      align: 'left',
    },
  },
);

const itemVariants = cva(
  // Base item styles
  'flex items-start gap-3 px-4 py-2.5 text-left transition-colors duration-150 cursor-pointer',
  {
    variants: {
      size: {
        sm: 'text-sm',
        md: 'text-sm',
        lg: 'text-base',
      },
      disabled: {
        true: 'cursor-not-allowed opacity-50',
        false: '',
      },
      destructive: {
        true: 'text-error-primary hover:bg-error-light hover:text-error-dark',
        false: 'text-neutral-900 hover:bg-trust-light hover:text-trust-deep',
      },
    },
    defaultVariants: {
      size: 'md',
      disabled: false,
      destructive: false,
    },
  },
);

const CheckIcon = () => (
  <Check className="w-4 h-4" aria-hidden="true" strokeWidth={1.5} />
);

const ChevronDownIcon = () => (
  <ChevronDown className="w-4 h-4 ml-1" aria-hidden="true" strokeWidth={1.5} />
);

export interface DropdownItem {
  /** Unique identifier for the item */
  id: string;
  /** Item label text */
  label: string;
  /** Optional icon to display before label */
  icon?: React.ReactNode;
  /** Optional description text below label */
  description?: string;
  /** Whether the item is disabled */
  disabled?: boolean;
  /** Highlight item as destructive action (red) */
  destructive?: boolean;
  /** Add a divider after this item */
  divider?: boolean;
  /** Section header label (creates visual grouping) */
  section?: string;
  /** Whether this item is currently selected */
  selected?: boolean;
  /** Optional callback for this specific item */
  onClick?: () => void;
}

export interface DropdownProps {
  /** Array of dropdown items */
  items: DropdownItem[];
  /** Trigger element (button, link, etc.) */
  trigger: React.ReactNode;
  /** Callback when an item is selected */
  onSelect?: (item: DropdownItem) => void;
  /** Dropdown alignment relative to trigger */
  align?: 'left' | 'right';
  /** Dropdown size */
  size?: 'sm' | 'md' | 'lg';
  /** Additional class names for the menu panel */
  className?: string;
  /** Additional class names for the trigger wrapper */
  triggerClassName?: string;
}

/**
 * Dropdown menu component for action lists and contextual menus.
 * Uses Headless UI Menu for full accessibility and keyboard navigation.
 *
 * @example
 * ```tsx
 * <Dropdown
 *   trigger={<Button>Actions</Button>}
 *   items={[
 *     { id: '1', label: 'Edit', icon: <EditIcon /> },
 *     { id: '2', label: 'Delete', destructive: true, icon: <TrashIcon /> },
 *   ]}
 *   onSelect={(item) => console.log('Selected:', item)}
 * />
 *
 * <Dropdown
 *   trigger={<Button>User Menu</Button>}
 *   align="right"
 *   items={[
 *     { id: '1', label: 'Profile', section: 'Account' },
 *     { id: '2', label: 'Settings', divider: true },
 *     { id: '3', label: 'Sign Out', destructive: true },
 *   ]}
 * />
 * ```
 */
export const Dropdown = React.forwardRef<HTMLDivElement, DropdownProps>(
  (
    {
      items,
      trigger,
      onSelect,
      align = 'left',
      size = 'md',
      className,
      triggerClassName,
    },
    ref,
  ) => {
    // Group items by section
    const groupedItems = React.useMemo(() => {
      const groups: { section?: string; items: DropdownItem[] }[] = [];
      let currentGroup: { section?: string; items: DropdownItem[] } = {
        items: [],
      };

      items.forEach((item, index) => {
        // Start new section if section property changes
        if (item.section && item.section !== currentGroup.section) {
          if (currentGroup.items.length > 0) {
            groups.push(currentGroup);
          }
          currentGroup = { section: item.section, items: [item] };
        } else {
          currentGroup.items.push(item);
        }

        // Close current group if divider or last item
        if (item.divider || index === items.length - 1) {
          groups.push(currentGroup);
          currentGroup = { items: [] };
        }
      });

      return groups;
    }, [items]);

    const handleItemClick = (item: DropdownItem) => {
      if (item.disabled) return;

      if (item.onClick) {
        item.onClick();
      }

      if (onSelect) {
        onSelect(item);
      }
    };

    return (
      <Menu as="div" className="relative inline-block text-left" ref={ref}>
        {({ open }) => (
          <>
            <Menu.Button
              className={cn('inline-flex items-center', triggerClassName)}
            >
              {typeof trigger === 'string' ? (
                <span className="inline-flex items-center">
                  {trigger}
                  <ChevronDownIcon />
                </span>
              ) : (
                trigger
              )}
            </Menu.Button>

            <Transition
              show={open}
              as={React.Fragment}
              enter="transition ease-out duration-200"
              enterFrom="opacity-0 scale-95"
              enterTo="opacity-100 scale-100"
              leave="transition ease-in duration-150"
              leaveFrom="opacity-100 scale-100"
              leaveTo="opacity-0 scale-95"
            >
              <Menu.Items
                className={cn(dropdownVariants({ size, align }), className)}
              >
                <div className="py-1">
                  {groupedItems.map((group, groupIndex) => (
                    <React.Fragment key={groupIndex}>
                      {/* Section header */}
                      {group.section && (
                        <div className="px-4 py-2 text-xs font-medium text-neutral-500">
                          {group.section}
                        </div>
                      )}

                      {/* Group items */}
                      {group.items.map((item) => (
                        <Menu.Item key={item.id} disabled={item.disabled}>
                          {({ active }) => (
                            <button
                              type="button"
                              onClick={() => handleItemClick(item)}
                              className={cn(
                                itemVariants({
                                  size,
                                  disabled: item.disabled,
                                  destructive: item.destructive,
                                }),
                                active && !item.disabled && 'bg-trust-light',
                                'w-full',
                              )}
                              disabled={item.disabled}
                            >
                              {/* Icon or checkmark */}
                              {item.selected ? (
                                <span className="flex-shrink-0 w-4 h-4 text-trust">
                                  <CheckIcon />
                                </span>
                              ) : item.icon ? (
                                <span className="flex-shrink-0 w-4 h-4">
                                  {item.icon}
                                </span>
                              ) : (
                                <span className="flex-shrink-0 w-4 h-4" />
                              )}

                              {/* Label and description */}
                              <div className="flex-1 min-w-0">
                                <div className="font-medium truncate">
                                  {item.label}
                                </div>
                                {item.description && (
                                  <div
                                    className={cn(
                                      'mt-0.5 text-xs text-neutral-500 truncate',
                                      item.destructive && 'text-error-primary',
                                    )}
                                  >
                                    {item.description}
                                  </div>
                                )}
                              </div>
                            </button>
                          )}
                        </Menu.Item>
                      ))}

                      {/* Divider between groups */}
                      {groupIndex < groupedItems.length - 1 && (
                        <div className="my-1 border-t border-neutral-200" />
                      )}
                    </React.Fragment>
                  ))}
                </div>
              </Menu.Items>
            </Transition>
          </>
        )}
      </Menu>
    );
  },
);

Dropdown.displayName = 'Dropdown';

export default Dropdown;
