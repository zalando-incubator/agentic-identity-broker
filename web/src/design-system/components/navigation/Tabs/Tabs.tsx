/**
 * Tabs Component
 *
 * Accessible tabbed navigation using Headless UI Tab component.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 3 variants: underline, pill, button
 * - 3 sizes: sm, md, lg
 * - Horizontal and vertical orientations
 * - Controlled and uncontrolled modes
 * - Optional icons
 * - Animated indicator/underline
 * - Full keyboard accessibility (arrow keys, Tab, Enter, Space)
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { Tab } from '@headlessui/react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';

export interface TabItem {
  /** Unique identifier for the tab */
  id: string;
  /** Display label */
  label: string;
  /** Whether tab is disabled */
  disabled?: boolean;
  /** Optional icon element */
  icon?: React.ReactNode;
}

const tabListVariants = cva('flex gap-1', {
  variants: {
    variant: {
      underline: 'border-b border-neutral-200',
      pill: 'bg-neutral-100 rounded-lg p-1',
      button: 'gap-2',
    },
    orientation: {
      horizontal: 'flex-row',
      vertical: 'flex-col',
    },
  },
  compoundVariants: [
    {
      variant: 'underline',
      orientation: 'vertical',
      className: 'border-b-0 border-r border-neutral-200',
    },
  ],
  defaultVariants: {
    variant: 'underline',
    orientation: 'horizontal',
  },
});

const tabButtonVariants = cva(
  'relative inline-flex items-center justify-center gap-2 font-medium transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-trust-hover disabled:opacity-50 disabled:cursor-not-allowed',
  {
    variants: {
      variant: {
        underline:
          'border-b-2 border-transparent hover:text-trust hover:border-neutral-300',
        pill: 'rounded-md hover:bg-white/60',
        button:
          'border border-neutral-300 rounded-md hover:border-neutral-400 hover:bg-neutral-50',
      },
      size: {
        sm: 'px-3 py-1.5 text-sm',
        md: 'px-4 py-2 text-base',
        lg: 'px-5 py-3 text-lg',
      },
      orientation: {
        horizontal: '',
        vertical: 'w-full',
      },
      selected: {
        true: '',
        false: '',
      },
    },
    compoundVariants: [
      // Underline variant selected states
      {
        variant: 'underline',
        selected: true,
        className: 'text-trust-deep border-trust-hover font-medium',
      },
      {
        variant: 'underline',
        selected: false,
        className: 'text-neutral-600',
      },
      // Pill variant selected states
      {
        variant: 'pill',
        selected: true,
        className: 'bg-white text-trust-deep shadow-sm font-medium',
      },
      {
        variant: 'pill',
        selected: false,
        className: 'text-neutral-700',
      },
      // Button variant selected states
      {
        variant: 'button',
        selected: true,
        className:
          'bg-trust-hover text-white border-trust-hover font-medium hover:bg-trust',
      },
      {
        variant: 'button',
        selected: false,
        className: 'bg-white text-neutral-700',
      },
      // Vertical orientation adjustments for underline
      {
        variant: 'underline',
        orientation: 'vertical',
        className: 'border-b-0 border-r-2',
      },
    ],
    defaultVariants: {
      variant: 'underline',
      size: 'md',
      orientation: 'horizontal',
      selected: false,
    },
  },
);

const tabPanelVariants = cva(
  'focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-trust-hover rounded-md',
  {
    variants: {
      size: {
        sm: 'mt-3',
        md: 'mt-4',
        lg: 'mt-6',
      },
      orientation: {
        horizontal: '',
        vertical: 'ml-6',
      },
    },
    compoundVariants: [
      {
        orientation: 'vertical',
        size: 'sm',
        className: 'ml-4 mt-0',
      },
      {
        orientation: 'vertical',
        size: 'md',
        className: 'ml-6 mt-0',
      },
      {
        orientation: 'vertical',
        size: 'lg',
        className: 'ml-8 mt-0',
      },
    ],
    defaultVariants: {
      size: 'md',
      orientation: 'horizontal',
    },
  },
);

export interface TabsProps extends Omit<
  React.HTMLAttributes<HTMLDivElement>,
  'onChange'
> {
  /** Array of tab items */
  tabs: TabItem[];
  /** Tab panel content - must match tabs array length */
  children: React.ReactNode[] | React.ReactNode;
  /** Visual style variant */
  variant?: 'underline' | 'pill' | 'button';
  /** Tab size */
  size?: 'sm' | 'md' | 'lg';
  /** Tab orientation */
  orientation?: 'horizontal' | 'vertical';
  /** Default selected tab ID (uncontrolled mode) */
  defaultTab?: string;
  /** Selected tab ID (controlled mode) */
  selectedTab?: string;
  /** Callback when tab changes */
  onTabChange?: (tabId: string) => void;
}

/**
 * Tabs component for organizing content into tabbed panels.
 * Supports both controlled and uncontrolled modes.
 *
 * @example
 * ```tsx
 * // Uncontrolled
 * <Tabs
 *   tabs={[
 *     { id: 'tab1', label: 'Tab 1' },
 *     { id: 'tab2', label: 'Tab 2' },
 *   ]}
 *   defaultTab="tab1"
 * >
 *   <div>Panel 1</div>
 *   <div>Panel 2</div>
 * </Tabs>
 *
 * // Controlled
 * <Tabs
 *   tabs={tabs}
 *   selectedTab={activeTab}
 *   onTabChange={setActiveTab}
 * >
 *   {panels}
 * </Tabs>
 * ```
 */
export const Tabs = React.forwardRef<HTMLDivElement, TabsProps>(
  (
    {
      tabs,
      children,
      variant = 'underline',
      size = 'md',
      orientation = 'horizontal',
      defaultTab,
      selectedTab,
      onTabChange,
      className,
      ...props
    },
    ref,
  ) => {
    // Convert children to array
    const childArray = React.Children.toArray(children);

    // Ensure children count matches tabs count
    if (childArray.length !== tabs.length) {
      console.warn(
        `Tabs: Number of children (${childArray.length}) does not match number of tabs (${tabs.length})`,
      );
    }

    // Find default tab index
    const defaultIndex = defaultTab
      ? tabs.findIndex((tab) => tab.id === defaultTab)
      : 0;

    // Find controlled tab index
    const selectedIndex = selectedTab
      ? tabs.findIndex((tab) => tab.id === selectedTab)
      : undefined;

    // Handle tab change
    const handleChange = (index: number) => {
      if (onTabChange && tabs[index]) {
        onTabChange(tabs[index].id);
      }
    };

    // Determine if controlled or uncontrolled
    const isControlled = selectedTab !== undefined;

    return (
      <div
        ref={ref}
        className={cn(
          'w-full',
          orientation === 'vertical' && 'flex',
          className,
        )}
        {...props}
      >
        <Tab.Group
          selectedIndex={isControlled ? selectedIndex : undefined}
          defaultIndex={!isControlled ? defaultIndex : undefined}
          onChange={handleChange}
          vertical={orientation === 'vertical'}
        >
          <Tab.List className={tabListVariants({ variant, orientation })}>
            {tabs.map((tab) => (
              <Tab
                key={tab.id}
                disabled={tab.disabled}
                className={({ selected }) =>
                  tabButtonVariants({ variant, size, orientation, selected })
                }
              >
                <>
                  {tab.icon && (
                    <span
                      className={cn(
                        'flex-shrink-0',
                        size === 'sm' && 'w-4 h-4',
                        size === 'md' && 'w-5 h-5',
                        size === 'lg' && 'w-6 h-6',
                      )}
                      aria-hidden="true"
                    >
                      {tab.icon}
                    </span>
                  )}
                  <span>{tab.label}</span>
                </>
              </Tab>
            ))}
          </Tab.List>

          <Tab.Panels className={cn(orientation === 'vertical' && 'flex-1')}>
            {childArray.map((child, index) => (
              <Tab.Panel
                key={tabs[index]?.id || index}
                className={tabPanelVariants({ size, orientation })}
              >
                {child}
              </Tab.Panel>
            ))}
          </Tab.Panels>
        </Tab.Group>
      </div>
    );
  },
);

Tabs.displayName = 'Tabs';

export default Tabs;
