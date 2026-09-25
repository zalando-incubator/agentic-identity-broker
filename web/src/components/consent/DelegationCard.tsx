/**
 * DelegationCard component displays a summary of an agent delegation.
 *
 * Shows:
 * - Agent logo and name
 * - Number of active service grants
 * - Last modified date
 * - Optional expiration date
 *
 * Performance: Memoized to prevent unnecessary re-renders in lists.
 */

import React, { memo } from 'react';
import { formatDistanceToNow, format } from 'date-fns';
import type { AgentDelegation } from '../../types/consent';
import { Card } from '@design-system/components/data-display/Card';
import { Stack } from '@design-system/components/layout/Stack';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { Button } from '@design-system/components/primitives/Button';

interface DelegationCardProps {
  /** Agent delegation data */
  delegation: AgentDelegation;
  /** Callback when card is clicked */
  onClick: () => void;
  /** Optional callback to revoke all access for this agent */
  onRevoke?: (agentId: string) => void;
}

/**
 * DelegationCard displays a clickable card with agent delegation summary.
 * Clicking the card navigates to the detailed grant management page.
 * Memoized for performance in large lists.
 */
function DelegationCardComponent({
  delegation,
  onClick,
  onRevoke,
}: DelegationCardProps) {
  const { displayName, logoUrl, activeGrantCount, lastModifiedAt, expiresAt } =
    delegation;

  // Format last modified time as relative (e.g., "2 days ago")
  const lastModifiedText = formatDistanceToNow(new Date(lastModifiedAt), {
    addSuffix: true,
  });

  // Format expiration date if present
  const expirationText = expiresAt
    ? `Expires ${format(new Date(expiresAt), 'MMM d, yyyy')}`
    : null;

  return (
    <Card
      padding="spacious"
      hover="none"
      clickable
      onClick={onClick}
      className="w-full focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
    >
      <Stack gap="md">
        {/* Agent logo, name, and arrow */}
        <Stack direction="row" gap="md" align="start">
          {/* Logo */}
          <Avatar
            src={logoUrl}
            alt={`${displayName} logo`}
            initials={displayName.charAt(0).toUpperCase()}
            shape="rounded"
            size="lg"
            className="flex-shrink-0"
          />

          {/* Agent info */}
          <Stack gap="xs" className="flex-1 min-w-0">
            <h3 className="text-lg font-semibold text-neutral-900 truncate">
              {displayName}
            </h3>
            <p className="text-sm text-neutral-600">
              {activeGrantCount === 1
                ? '1 service'
                : `${activeGrantCount} services`}
            </p>
          </Stack>

          {/* Arrow icon */}
          <div className="flex-shrink-0 text-neutral-400">
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 5l7 7-7 7"
              />
            </svg>
          </div>
        </Stack>

        {/* Metadata row */}
        <Stack direction="row" justify="space-between" align="center">
          <span className="text-xs text-neutral-500" data-screenshot-dynamic>
            Updated {lastModifiedText}
          </span>
          <Stack direction="row" gap="sm" align="center">
            {expirationText && (
              <Stack direction="row" gap="xs" align="center">
                <svg
                  className="w-4 h-4 text-neutral-500"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  aria-hidden="true"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                <span className="text-xs text-neutral-500" data-screenshot-dynamic>
                  {expirationText}
                </span>
              </Stack>
            )}
            {onRevoke && (
              <Button
                variant="danger"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  onRevoke(delegation.agentId);
                }}
                aria-label={`Revoke access for ${displayName}`}
              >
                Revoke
              </Button>
            )}
          </Stack>
        </Stack>
      </Stack>
    </Card>
  );
}

/**
 * Memoized DelegationCard component.
 * Only re-renders if delegation data or onClick changes.
 */
export const DelegationCard = memo(
  DelegationCardComponent,
  (prevProps, nextProps) => {
    // Custom comparison: only re-render if delegation, onClick, or onRevoke changed
    return (
      prevProps.delegation.agentId === nextProps.delegation.agentId &&
      prevProps.delegation.lastModifiedAt ===
        nextProps.delegation.lastModifiedAt &&
      prevProps.delegation.activeGrantCount ===
        nextProps.delegation.activeGrantCount &&
      prevProps.delegation.expiresAt === nextProps.delegation.expiresAt &&
      prevProps.onClick === nextProps.onClick &&
      prevProps.onRevoke === nextProps.onRevoke
    );
  },
);

DelegationCard.displayName = 'DelegationCard';

export default DelegationCard;
