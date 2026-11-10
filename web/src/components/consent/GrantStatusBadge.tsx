/**
 * GrantStatusBadge component displays the status of a grant.
 *
 * Features:
 * - Color-coded badge (active: success, expired: neutral, pending: warning)
 * - Expiration date display
 * - Icon indicators
 * - Accessible with proper ARIA labels
 * Uses the design system Badge component for consistency
 */

import React from 'react';
import { format, isPast } from 'date-fns';
import { Badge } from '@design-system/components/primitives/Badge';
import { CircleCheck, CircleX, Clock } from 'lucide-react';

type GrantStatus = 'active' | 'expired' | 'pending';

interface GrantStatusBadgeProps {
  /** Grant status */
  status: GrantStatus;
  /** Optional expiration date */
  expiresAt?: Date | string | null;
}

/**
 * GrantStatusBadge displays a colored badge indicating grant status.
 * Shows expiration date if available. Uses design system Badge component.
 */
export function GrantStatusBadge({ status, expiresAt }: GrantStatusBadgeProps) {
  // Parse expiration date if provided
  const expirationDate = expiresAt ? new Date(expiresAt) : null;
  const isExpired = expirationDate ? isPast(expirationDate) : false;

  // Override status if expiration date is in the past
  const effectiveStatus: GrantStatus = isExpired ? 'expired' : status;

  // Map status to design system Badge variant and icon
  const getStatusConfig = () => {
    switch (effectiveStatus) {
      case 'active':
        return {
          variant: 'success' as const,
          label: 'Active',
          icon: (
            <CircleCheck
              className="w-full h-full"
              aria-hidden="true"
              strokeWidth={1.5}
            />
          ),
        };
      case 'expired':
        return {
          variant: 'neutral' as const,
          label: 'Expired',
          icon: (
            <CircleX
              className="w-full h-full"
              aria-hidden="true"
              strokeWidth={1.5}
            />
          ),
        };
      case 'pending':
        return {
          variant: 'warning' as const,
          label: 'Pending',
          icon: (
            <Clock
              className="w-full h-full"
              aria-hidden="true"
              strokeWidth={1.5}
            />
          ),
        };
      default:
        return {
          variant: 'neutral' as const,
          label: 'Unknown',
          icon: null,
        };
    }
  };

  const config = getStatusConfig();

  // Format expiration text
  const getExpirationText = () => {
    if (!expirationDate) return null;

    if (isExpired) {
      return `Expired ${format(expirationDate, 'MMM d, yyyy')}`;
    }

    return `Expires ${format(expirationDate, 'MMM d, yyyy')}`;
  };

  const expirationText = getExpirationText();

  return (
    <div className="inline-flex items-center gap-2">
      {/* Status badge using design system */}
      <Badge
        variant={config.variant}
        size="md"
        showDot={effectiveStatus === 'active'}
        iconBefore={config.icon}
        role="status"
      >
        {config.label}
      </Badge>

      {/* Expiration date */}
      {expirationText && (
        <span
          data-screenshot-dynamic
          className={`text-xs ${isExpired ? 'text-neutral-500 line-through' : 'text-neutral-600'}`}
          aria-label={expirationText}
        >
          {expirationText}
        </span>
      )}
    </div>
  );
}

export default GrantStatusBadge;
