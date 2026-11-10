/**
 * Progress Component
 *
 * Visual progress indicator for displaying task/operation completion.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Linear and circular progress variants
 * - Determinate (0-100%) and indeterminate (animated) modes
 * - 4 status variants (default, success, warning, error)
 * - 3 size variants (sm, md, lg)
 * - Optional label and value display
 * - Custom value formatting (percentage or custom text)
 * - Striped animation effect for linear progress
 * - Custom height support for linear
 * - Smooth animations via CSS transitions
 * - WCAG 2.1 AA accessible with proper ARIA
 */

import React from 'react';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const progressContainerVariants = cva('relative', {
  variants: {
    type: {
      linear: 'w-full overflow-hidden rounded-full bg-neutral-200',
      circular: 'inline-flex items-center justify-center',
    },
    size: {
      sm: '',
      md: '',
      lg: '',
    },
  },
  compoundVariants: [
    // Linear sizes (height)
    { type: 'linear', size: 'sm', class: 'h-2' },
    { type: 'linear', size: 'md', class: 'h-3' },
    { type: 'linear', size: 'lg', class: 'h-4' },
  ],
  defaultVariants: {
    type: 'linear',
    size: 'md',
  },
});

const progressBarVariants = cva('h-full transition-all duration-500 ease-out', {
  variants: {
    variant: {
      default: 'bg-trust-hover',
      success: 'bg-success-primary',
      warning: 'bg-warning-primary',
      error: 'bg-error-primary',
    },
    striped: {
      true: 'bg-gradient-to-r bg-[length:1rem_1rem] animate-[progress-stripes_1s_linear_infinite]',
      false: '',
    },
    indeterminate: {
      true: 'animate-[progress-indeterminate_1.5s_ease-in-out_infinite]',
      false: '',
    },
  },
  compoundVariants: [
    // Striped patterns per variant
    {
      variant: 'default',
      striped: true,
      class: 'from-trust-hover via-trust to-trust-hover',
    },
    {
      variant: 'success',
      striped: true,
      class: 'from-success-primary via-success-hover to-success-primary',
    },
    {
      variant: 'warning',
      striped: true,
      class: 'from-warning-primary via-warning-hover to-warning-primary',
    },
    {
      variant: 'error',
      striped: true,
      class: 'from-error-primary via-error-hover to-error-primary',
    },
  ],
  defaultVariants: {
    variant: 'default',
    striped: false,
    indeterminate: false,
  },
});

const circularSizeMap = {
  sm: { size: 40, strokeWidth: 4 },
  md: { size: 64, strokeWidth: 6 },
  lg: { size: 96, strokeWidth: 8 },
};

const circularVariantColorMap = {
  default: 'stroke-trust-hover',
  success: 'stroke-success-primary',
  warning: 'stroke-warning-primary',
  error: 'stroke-error-primary',
};

export interface ProgressProps extends Omit<
  React.HTMLAttributes<HTMLDivElement>,
  'children'
> {
  /** Progress value (0-100). If omitted, shows indeterminate state */
  value?: number;
  /** Visual variant for semantic meaning */
  variant?: 'default' | 'success' | 'warning' | 'error';
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
  /** Progress type: linear bar or circular */
  type?: 'linear' | 'circular';
  /** Optional label displayed above progress */
  label?: string;
  /** Whether to show value text */
  showValue?: boolean;
  /** Value format: percentage or custom */
  valueFormat?: 'percentage' | 'custom';
  /** Custom value text (overrides percentage) */
  customValue?: string | React.ReactNode;
  /** Force indeterminate state (animated) */
  indeterminate?: boolean;
  /** Show striped animation effect (linear only) */
  striped?: boolean;
  /** Custom height for linear progress (overrides size) */
  height?: number | string;
}

/**
 * Progress component for displaying task/operation completion.
 * Supports both linear and circular variants with customizable appearance.
 *
 * @example
 * ```tsx
 * // Basic determinate progress
 * <Progress value={45} />
 *
 * // With label and value display
 * <Progress
 *   value={75}
 *   label="Upload progress"
 *   showValue
 * />
 *
 * // Indeterminate loading state
 * <Progress indeterminate />
 *
 * // Success state with circular variant
 * <Progress
 *   type="circular"
 *   value={100}
 *   variant="success"
 * />
 *
 * // Striped animation
 * <Progress
 *   value={60}
 *   striped
 *   variant="warning"
 * />
 *
 * // Custom value text
 * <Progress
 *   value={30}
 *   showValue
 *   customValue="3 of 10 items"
 * />
 * ```
 */
export const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
  (
    {
      value,
      variant = 'default',
      size = 'md',
      type = 'linear',
      label,
      showValue = false,
      valueFormat = 'percentage',
      customValue,
      indeterminate = false,
      striped = false,
      height,
      className,
      ...props
    },
    ref,
  ) => {
    // Normalize value between 0-100
    const normalizedValue =
      indeterminate || value === undefined
        ? 0
        : Math.min(100, Math.max(0, value));

    // Determine if we should show indeterminate state
    const isIndeterminate = indeterminate || value === undefined;

    // Calculate value text
    const getValueText = () => {
      if (customValue) return customValue;
      if (valueFormat === 'percentage')
        return `${Math.round(normalizedValue)}%`;
      return normalizedValue;
    };

    // Linear progress variant
    if (type === 'linear') {
      return (
        <div ref={ref} className={cn('w-full', className)} {...props}>
          {/* Label and value row */}
          {(label || showValue) && (
            <div className="flex items-center justify-between mb-2">
              {label && (
                <span className="text-sm font-medium text-neutral-700">
                  {label}
                </span>
              )}
              {showValue && (
                <span className="text-sm font-medium text-neutral-600">
                  {getValueText()}
                </span>
              )}
            </div>
          )}

          {/* Progress bar container */}
          <div
            className={cn(progressContainerVariants({ type, size }), className)}
            style={height ? { height } : undefined}
            role="progressbar"
            aria-valuenow={isIndeterminate ? undefined : normalizedValue}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={label || 'Progress'}
            aria-busy={isIndeterminate}
          >
            {/* Progress bar fill */}
            <div
              className={cn(
                progressBarVariants({
                  variant,
                  striped,
                  indeterminate: isIndeterminate,
                }),
              )}
              style={{
                width: isIndeterminate ? '30%' : `${normalizedValue}%`,
              }}
            />
          </div>
        </div>
      );
    }

    // Circular progress variant
    const { size: svgSize, strokeWidth } = circularSizeMap[size];
    const radius = (svgSize - strokeWidth) / 2;
    const circumference = 2 * Math.PI * radius;
    const offset = isIndeterminate
      ? circumference * 0.75 // Show partial arc
      : circumference - (normalizedValue / 100) * circumference;

    return (
      <div
        ref={ref}
        className={cn('inline-flex flex-col items-center gap-2', className)}
        {...props}
      >
        {/* Label above circle */}
        {label && (
          <span className="text-sm font-medium text-neutral-700">{label}</span>
        )}

        {/* SVG circular progress */}
        <div className="relative inline-flex items-center justify-center">
          <svg
            width={svgSize}
            height={svgSize}
            viewBox={`0 0 ${svgSize} ${svgSize}`}
            role="progressbar"
            aria-valuenow={isIndeterminate ? undefined : normalizedValue}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={label || 'Progress'}
            aria-busy={isIndeterminate}
            className={cn(isIndeterminate && 'animate-spin')}
            style={isIndeterminate ? { animationDuration: '1.5s' } : undefined}
          >
            {/* Background circle */}
            <circle
              cx={svgSize / 2}
              cy={svgSize / 2}
              r={radius}
              fill="none"
              stroke="currentColor"
              strokeWidth={strokeWidth}
              className="text-neutral-200"
            />

            {/* Progress arc */}
            <circle
              cx={svgSize / 2}
              cy={svgSize / 2}
              r={radius}
              fill="none"
              strokeWidth={strokeWidth}
              strokeLinecap="round"
              className={cn(
                'transition-all duration-500 ease-out',
                circularVariantColorMap[variant],
              )}
              style={{
                strokeDasharray: circumference,
                strokeDashoffset: offset,
                transform: 'rotate(-90deg)',
                transformOrigin: '50% 50%',
              }}
            />
          </svg>

          {/* Center value text */}
          {showValue && !isIndeterminate && (
            <div className="absolute inset-0 flex items-center justify-center">
              <span
                className={cn(
                  'font-medium text-neutral-900',
                  size === 'sm' && 'text-xs',
                  size === 'md' && 'text-sm',
                  size === 'lg' && 'text-base',
                )}
              >
                {getValueText()}
              </span>
            </div>
          )}
        </div>
      </div>
    );
  },
);

Progress.displayName = 'Progress';

export default Progress;
