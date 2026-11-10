/**
 * EmptyState component displays a centered message when no data is available.
 *
 * Features:
 * - Optional icon/illustration
 * - Title and description text
 * - Optional call-to-action button
 * - Centered layout with proper spacing
 */

import React, { ReactNode } from 'react';
import { Inbox } from 'lucide-react';
import { Button } from '@design-system/components/primitives/Button';

interface EmptyStateProps {
  /** Optional icon or image element to display */
  icon?: ReactNode;
  /** Main heading text */
  title: string;
  /** Descriptive text explaining the empty state */
  description: string;
  /** Optional action button configuration */
  action?: {
    label: string;
    onClick: () => void;
  };
}

/**
 * EmptyState displays a user-friendly message when there's no content to show.
 * Commonly used for empty lists, search results, etc.
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
}: EmptyStateProps) {
  return (
    <div className="card p-12 text-center">
      {/* Icon/Illustration */}
      {icon ? (
        <div className="flex justify-center">{icon}</div>
      ) : (
        <Inbox
          className="mx-auto h-12 w-12 text-neutral-400"
          aria-hidden="true"
          strokeWidth={1.5}
        />
      )}

      {/* Title */}
      <h3 className="mt-4 text-lg font-medium text-neutral-900">{title}</h3>

      {/* Description */}
      <p className="mt-2 text-sm text-neutral-600 max-w-md mx-auto">
        {description}
      </p>

      {/* Optional action button */}
      {action && (
        <div className="mt-6">
          <Button size="sm" onClick={action.onClick}>
            {action.label}
          </Button>
        </div>
      )}
    </div>
  );
}

export default EmptyState;
