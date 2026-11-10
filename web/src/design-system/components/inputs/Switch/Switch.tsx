/**
 * Switch Component
 *
 * Accessible toggle switch using Headless UI.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 3 sizes: sm, md, lg
 * - Smooth animations with semantic colors
 * - Full keyboard accessibility
 * - Disabled state support
 * - Label support
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { Switch as HeadlessSwitch } from '@headlessui/react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const switchContainerVariants = cva(
  'relative inline-flex flex-shrink-0 items-center rounded-full transition-colors duration-150 focus:outline-none focus:ring-2 focus:ring-offset-2',
  {
    variants: {
      size: {
        sm: 'h-5 w-8 focus:ring-offset-1',
        md: 'h-6 w-11 focus:ring-offset-2',
        lg: 'h-7 w-14 focus:ring-offset-2',
      },
      variant: {
        primary:
          'focus:ring-accent data-[checked=true]:bg-accent data-[checked=false]:bg-neutral-300',
        success:
          'focus:ring-success-primary data-[checked=true]:bg-success-primary data-[checked=false]:bg-neutral-300',
      },
    },
    defaultVariants: {
      size: 'md',
      variant: 'primary',
    },
  },
);

const switchThumbVariants = cva(
  'inline-block transform rounded-full bg-white shadow-sm transition-transform duration-200 ease-in-out',
  {
    variants: {
      size: {
        sm: 'h-4 w-4 data-[checked=true]:translate-x-3.5 data-[checked=false]:translate-x-0.5',
        md: 'h-5 w-5 data-[checked=true]:translate-x-5 data-[checked=false]:translate-x-0.5',
        lg: 'h-6 w-6 data-[checked=true]:translate-x-6.5 data-[checked=false]:translate-x-0.5',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  },
);

export interface SwitchProps
  extends
    Omit<React.HTMLAttributes<HTMLDivElement>, 'onChange'>,
    VariantProps<typeof switchContainerVariants> {
  /** Whether the switch is checked */
  checked: boolean;
  /** Callback when switch state changes */
  onChange: (checked: boolean) => void;
  /** Optional label text */
  label?: string;
  /** Description text displayed below label */
  description?: string;
  /** Whether the switch is disabled */
  disabled?: boolean;
  /** Unique identifier for form association */
  id?: string;
}

/**
 * Switch component for binary on/off controls.
 * Wraps Headless UI Switch with design system styling and semantic tokens.
 *
 * @example
 * ```tsx
 * <Switch
 *   checked={enabled}
 *   onChange={setEnabled}
 *   label="Enable notifications"
 * />
 *
 * <Switch
 *   checked={grant}
 *   onChange={setGrant}
 *   label="Grant access"
 *   description="Allow this agent to read your emails"
 *   size="lg"
 * />
 * ```
 */
export const Switch = React.forwardRef<HTMLDivElement, SwitchProps>(
  (
    {
      size = 'md',
      variant = 'primary',
      checked,
      onChange,
      label,
      description,
      disabled = false,
      id,
      className,
      ...props
    },
    ref,
  ) => {
    return (
      <HeadlessSwitch.Group>
        <div
          ref={ref}
          className={cn('flex items-start gap-3', className)}
          {...props}
        >
          {/* Switch control */}
          <div className="flex items-center pt-0.5">
            <HeadlessSwitch
              checked={checked}
              onChange={onChange}
              disabled={disabled}
              id={id}
              data-checked={checked}
              className={cn(
                switchContainerVariants({ size, variant }),
                disabled && 'opacity-50 cursor-not-allowed',
              )}
            >
              <span
                className={cn(switchThumbVariants({ size }), 'shadow-md')}
                data-checked={checked}
              />
            </HeadlessSwitch>
          </div>

          {/* Label and description */}
          {(label || description) && (
            <div className="flex flex-col gap-1">
              {label && (
                <HeadlessSwitch.Label
                  className={cn(
                    'text-sm font-medium',
                    disabled
                      ? 'text-neutral-500 cursor-not-allowed'
                      : 'text-neutral-900 cursor-pointer',
                  )}
                >
                  {label}
                </HeadlessSwitch.Label>
              )}

              {description && (
                <p className="text-xs text-neutral-600">{description}</p>
              )}
            </div>
          )}
        </div>
      </HeadlessSwitch.Group>
    );
  },
);

Switch.displayName = 'Switch';

export default Switch;
