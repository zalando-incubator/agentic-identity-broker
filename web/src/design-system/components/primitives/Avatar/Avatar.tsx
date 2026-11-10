/**
 * Avatar Component
 *
 * Displays user or agent profile pictures, initials, or fallback icons.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - 5 sizes: xs, sm, md, lg, xl
 * - Image support with automatic fallback
 * - Initials display (1-2 characters)
 * - Icon fallback
 * - Status indicator (online, offline, away, busy)
 * - Rounded or square shape
 * - WCAG 2.1 AA compliant
 */

import React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { User } from 'lucide-react';

const avatarVariants = cva(
  'inline-flex items-center justify-center overflow-hidden bg-neutral-100 text-neutral-700 font-medium relative',
  {
    variants: {
      size: {
        xs: 'w-6 h-6 text-xs',
        sm: 'w-8 h-8 text-sm',
        md: 'w-10 h-10 text-base',
        lg: 'w-12 h-12 text-lg',
        xl: 'w-16 h-16 text-2xl',
      },
      shape: {
        circle: 'rounded-full',
        rounded: 'rounded-md',
        square: 'rounded-none',
      },
    },
    defaultVariants: {
      size: 'md',
      shape: 'circle',
    },
  },
);

const statusIndicatorVariants = cva(
  'absolute bottom-0 right-0 rounded-full border-2 border-white',
  {
    variants: {
      size: {
        xs: 'w-1.5 h-1.5',
        sm: 'w-2 h-2',
        md: 'w-2.5 h-2.5',
        lg: 'w-3 h-3',
        xl: 'w-4 h-4',
      },
      status: {
        online: 'bg-success-primary',
        offline: 'bg-neutral-400',
        away: 'bg-warning-primary',
        busy: 'bg-error-primary',
      },
    },
  },
);

export interface AvatarProps
  extends
    Omit<React.HTMLAttributes<HTMLDivElement>, 'children'>,
    VariantProps<typeof avatarVariants> {
  /** Image source URL */
  src?: string;
  /** Alt text for the image */
  alt?: string;
  /** Initials to display (1-2 characters) */
  initials?: string;
  /** Fallback icon element */
  fallbackIcon?: React.ReactNode;
  /** Status indicator */
  status?: 'online' | 'offline' | 'away' | 'busy';
  /** Callback when image fails to load */
  onError?: () => void;
}

/**
 * Avatar component for displaying profile pictures or initials.
 * Automatically falls back to initials or icon if image fails to load.
 *
 * @example
 * ```tsx
 * <Avatar src="/profile.jpg" alt="John Doe" />
 *
 * <Avatar initials="JD" status="online" />
 *
 * <Avatar fallbackIcon={<UserIcon />} size="lg" />
 *
 * <Avatar src="/agent.png" alt="AI Agent" shape="rounded" />
 * ```
 */
export const Avatar = React.forwardRef<HTMLDivElement, AvatarProps>(
  (
    {
      size,
      shape,
      className,
      src,
      alt = '',
      initials,
      fallbackIcon,
      status,
      onError,
      ...props
    },
    ref,
  ) => {
    const [imageError, setImageError] = React.useState(false);
    const [imageLoaded, setImageLoaded] = React.useState(false);

    const handleImageError = () => {
      setImageError(true);
      onError?.();
    };

    const handleImageLoad = () => {
      setImageLoaded(true);
    };

    // Determine what content to show
    const showImage = src && !imageError;
    const showInitials = !showImage && initials;
    const showFallbackIcon = !showImage && !showInitials && fallbackIcon;

    // Process initials to max 2 characters
    const displayInitials = initials ? initials.slice(0, 2).toUpperCase() : '';

    return (
      <div
        ref={ref}
        className={cn(avatarVariants({ size, shape }), className)}
        {...props}
      >
        {/* Image */}
        {showImage && (
          <img
            src={src}
            alt={alt}
            className={cn(
              'w-full h-full object-cover',
              !imageLoaded && 'opacity-0',
            )}
            onError={handleImageError}
            onLoad={handleImageLoad}
          />
        )}

        {/* Initials */}
        {showInitials && <span>{displayInitials}</span>}

        {/* Fallback Icon */}
        {showFallbackIcon && (
          <span className="w-1/2 h-1/2 text-neutral-400">{fallbackIcon}</span>
        )}

        {/* Default fallback icon (person) */}
        {!showImage && !showInitials && !showFallbackIcon && (
          <User
            className="w-1/2 h-1/2 text-neutral-400"
            aria-hidden="true"
            strokeWidth={1.5}
          />
        )}

        {/* Status indicator */}
        {status && (
          <span
            className={cn(statusIndicatorVariants({ size, status }))}
            aria-label={`Status: ${status}`}
          />
        )}
      </div>
    );
  },
);

Avatar.displayName = 'Avatar';

export default Avatar;
