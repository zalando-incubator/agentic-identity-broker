/**
 * Card Component
 *
 * Flexible container component for displaying grouped content.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Customizable padding (none, sm, md, lg)
 * - 2 border variants (subtle ring, bordered)
 * - 2 hover states (none, lift)
 * - Optional clickable/interactive state
 * - Header, body, and footer sections
 * - Customizable background colors
 * - Optional dividers between sections
 * - Icon slot in header
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { Stack } from '@design-system/components/layout/Stack';
import { Divider } from '@design-system/components/primitives/Divider';

const cardVariants = cva(
  // Base styles - applied to all variants
  'rounded-xl transition-colors duration-150',
  {
    variants: {
      padding: {
        // None: No padding - full control to content
        none: '',

        // Compact: 16px - Dense cards
        compact: 'p-4',

        // Default: 24px - Standard spacing (default)
        default: 'p-6',

        // Spacious: 32px - Generous spacing
        spacious: 'p-8',
      },
      border: {
        // Subtle: Hairline border, the default containment
        subtle: 'border border-neutral-200',

        // Bordered: Stronger hairline for emphasis
        bordered: 'border border-neutral-300',
      },
      hover: {
        // None: Static card
        none: '',

        // Lift: The hairline takes the accent on hover. No shadow, no movement.
        lift: 'hover:border-accent',
      },
      backgroundColor: {
        // White: Pure white background (default)
        white: 'bg-white',

        // Gray-50: Subtle gray background
        'neutral-50': 'bg-neutral-50',

        // Blue-50: Light blue background for highlights
        'blue-50': 'bg-info-light',
      },
    },
    defaultVariants: {
      padding: 'default',
      border: 'subtle',
      hover: 'none',
      backgroundColor: 'white',
    },
  },
);

const headerVariants = cva('', {
  variants: {
    padding: {
      none: '',
      compact: 'pb-3',
      default: 'pb-4',
      spacious: 'pb-6',
    },
  },
  defaultVariants: {
    padding: 'default',
  },
});

const footerVariants = cva('', {
  variants: {
    padding: {
      none: '',
      compact: 'pt-3',
      default: 'pt-4',
      spacious: 'pt-6',
    },
  },
  defaultVariants: {
    padding: 'default',
  },
});

export interface CardProps
  extends
    React.HTMLAttributes<HTMLElement>,
    Omit<VariantProps<typeof cardVariants>, 'padding'> {
  /** Main content of the card */
  children: React.ReactNode;
  /** Optional header section */
  header?: React.ReactNode;
  /** Optional footer section */
  footer?: React.ReactNode;
  /** Padding size for the card */
  padding?: 'none' | 'compact' | 'default' | 'spacious';
  /** Whether the card is clickable/interactive */
  clickable?: boolean;
  /** Click handler for clickable cards */
  onClick?: () => void;
  /** Optional icon in the header */
  headerIcon?: React.ReactNode;
  /** Show dividers between sections */
  divider?: boolean;
  /** Use semantic HTML tag */
  as?: 'div' | 'section' | 'article';
}

/**
 * Card component for displaying grouped content with consistent styling.
 * Supports headers, footers, various padding and border options, and interactive states.
 *
 * @example
 * ```tsx
 * // Simple card with content
 * <Card>
 *   <p>Card content goes here</p>
 * </Card>
 *
 * // Card with header and footer
 * <Card
 *   header={<h2>Card Title</h2>}
 *   footer={<Button>Action</Button>}
 * >
 *   <p>Card body content</p>
 * </Card>
 *
 * // Clickable card with lift effect
 * <Card hover="lift" clickable onClick={handleClick}>
 *   <p>Click me!</p>
 * </Card>
 *
 * // Card with icon and dividers
 * <Card
 *   header={<h3>Settings</h3>}
 *   headerIcon={<SettingsIcon />}
 *   divider
 *   padding="lg"
 * >
 *   <p>Configuration options</p>
 * </Card>
 *
 * // Card with custom background
 * <Card backgroundColor="blue-50" border="bordered">
 *   <p>Highlighted content</p>
 * </Card>
 * ```
 */
export const Card = React.forwardRef<HTMLElement, CardProps>(
  (
    {
      children,
      header,
      footer,
      padding = 'md',
      border,
      hover,
      backgroundColor,
      clickable = false,
      onClick,
      headerIcon,
      divider = false,
      className,
      as: Component = 'div',
      ...props
    },
    ref,
  ) => {
    const hasHeader = Boolean(header || headerIcon);
    const hasFooter = Boolean(footer);
    const isInteractive = clickable || Boolean(onClick);

    // Apply padding 'none' to the outer card if we have header/footer with padding
    // Otherwise, apply the padding to the card itself
    const outerPadding = (hasHeader || hasFooter ? 'none' : padding) as
      | 'none'
      | 'default'
      | 'compact'
      | 'spacious';
    const innerPadding = (hasHeader || hasFooter ? padding : 'none') as
      | 'none'
      | 'default'
      | 'compact'
      | 'spacious';

    return (
      <Component
        ref={ref as React.Ref<HTMLDivElement>}
        className={cn(
          cardVariants({
            padding: outerPadding,
            border,
            hover,
            backgroundColor,
          }),
          isInteractive && 'cursor-pointer',
          className,
        )}
        onClick={onClick}
        role={isInteractive ? 'button' : undefined}
        tabIndex={isInteractive ? 0 : undefined}
        onKeyDown={
          isInteractive && onClick
            ? (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onClick();
                }
              }
            : undefined
        }
        {...props}
      >
        <Stack gap="xs">
          {/* Header Section */}
          {hasHeader && (
            <>
              <div
                className={cn(
                  headerVariants({ padding: innerPadding }),
                  innerPadding !== 'none' && 'px-4',
                )}
              >
                {headerIcon ? (
                  <Stack direction="row" gap="sm" align="center">
                    <span className="flex items-center text-neutral-700">
                      {headerIcon}
                    </span>
                    {header}
                  </Stack>
                ) : (
                  header
                )}
              </div>
              {divider && <Divider variant="subtle" />}
            </>
          )}

          {/* Body Section */}
          <div
            className={cn(
              innerPadding !== 'none' &&
                (innerPadding === 'compact'
                  ? 'p-4'
                  : innerPadding === 'default'
                    ? 'p-6'
                    : 'p-8'),
            )}
          >
            {children}
          </div>

          {/* Footer Section */}
          {hasFooter && (
            <>
              {divider && <Divider variant="subtle" />}
              <div
                className={cn(
                  footerVariants({ padding: innerPadding }),
                  innerPadding !== 'none' && 'px-4',
                )}
              >
                {footer}
              </div>
            </>
          )}
        </Stack>
      </Component>
    );
  },
);

Card.displayName = 'Card';

export default Card;
