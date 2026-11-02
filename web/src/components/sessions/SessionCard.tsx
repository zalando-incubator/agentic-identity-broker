/**
 * SessionCard Component
 *
 * Displays a single OAuth2 session with status information, metadata, and actions.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Session status badge (Active, Access Token Expired, Reauthentication required)
 * - Encryption indicator
 * - Dependent agent count
 * - Session initiation timestamp
 * - OAuth2 scopes display
 * - Login, Refresh, and Terminate actions
 * - WCAG 2.1 AA compliant
 */

import React, { useMemo } from 'react';
import { Card } from '@design-system/components/data-display/Card';
import { Button } from '@design-system/components/primitives/Button';
import { Badge } from '@design-system/components/primitives/Badge';
import { StatusIndicator } from '@design-system/components/data-display/StatusIndicator';
import { Stack } from '@design-system/components/layout/Stack';
import type { SessionSummary } from '@services/api/sessions';

interface SessionCardProps {
  /** Session data to display */
  session: SessionSummary;
  /** Callback when user clicks terminate button */
  onTerminate: (serviceId: string) => void;
  /** Callback when user clicks view details (optional) */
  onViewDetails?: (serviceId: string) => void;
  /** Callback when user clicks the refresh button (force token refresh) */
  onRefresh?: (serviceId: string) => void;
  /** Callback when user needs to authenticate with the service again */
  onLogin?: (serviceId: string) => void;
  /** Whether the refresh action is currently loading for this card */
  refreshing?: boolean;
  /** Whether card actions are currently loading */
  loading?: boolean;
}

/**
 * SessionCard displays a single OAuth2 session with status information.
 *
 * @example
 * ```tsx
 * <SessionCard
 *   session={session}
 *   onTerminate={handleTerminate}
 *   onViewDetails={handleViewDetails}
 *   loading={isTerminating}
 * />
 * ```
 */
export const SessionCard: React.FC<SessionCardProps> = ({
  session,
  onTerminate,
  onViewDetails,
  onRefresh,
  onLogin,
  refreshing = false,
  loading = false,
}) => {
  // Determine status and color variant
  const { status, variant } = useMemo(() => {
    if (session.requires_reauthentication) {
      return { status: 'Reauthentication required', variant: 'error' as const };
    }
    if (session.access_token_expired) {
      return { status: 'Access Token Expired', variant: 'warning' as const };
    }

    return { status: 'Active', variant: 'success' as const };
  }, [session]);

  // Format initiation date using the user's browser locale
  const initiatedDate = useMemo(() => {
    return new Intl.DateTimeFormat(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(session.initiated_at));
  }, [session.initiated_at]);

  return (
    <Card padding="default" border="subtle" hover="lift">
      {/* Header: Service name and Terminate button */}
      <div className="mb-3 flex items-start justify-between gap-2">
        <div className="flex-1">
          <h3 className="m-0 text-lg font-semibold text-neutral-900">
            {session.service_display_name}
          </h3>
        </div>
        <div className="flex items-center gap-2">
          {onViewDetails && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onViewDetails(session.service_id)}
              disabled={loading || refreshing}
              className="flex-shrink-0"
            >
              View Details
            </Button>
          )}
          {onLogin && session.requires_reauthentication && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onLogin(session.service_id)}
              disabled={loading || refreshing}
              className="flex-shrink-0"
            >
              Login
            </Button>
          )}
          {onRefresh &&
            session.has_refresh_token &&
            !session.requires_reauthentication && (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => onRefresh(session.service_id)}
                isLoading={refreshing}
                disabled={loading || refreshing}
                className="flex-shrink-0"
              >
                Refresh
              </Button>
            )}
          <Button
            variant="danger"
            size="sm"
            onClick={() => onTerminate(session.service_id)}
            isLoading={loading}
            disabled={loading || refreshing}
            className="flex-shrink-0"
          >
            Terminate
          </Button>
        </div>
      </div>

      {/* Status */}
      <div className="mb-4 flex items-center justify-between gap-2">
        <Badge variant={variant} showDot>
          {status}
        </Badge>
      </div>

      {/* Body: Session metadata */}
      <Stack gap="sm">
        {/* Encryption indicator */}
        <StatusIndicator
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 20 20"
              fill="currentColor"
              className="w-4 h-4"
            >
              <path
                fillRule="evenodd"
                d="M10 1a4.5 4.5 0 00-4.5 4.5V9H5a2 2 0 00-2 2v6a2 2 0 002 2h10a2 2 0 002-2v-6a2 2 0 00-2-2h-.5V5.5A4.5 4.5 0 0010 1zm3 8V5.5a3 3 0 10-6 0V9h6z"
                clipRule="evenodd"
              />
            </svg>
          }
          label={session.is_encrypted ? 'Encrypted' : 'Not Encrypted'}
          variant={session.is_encrypted ? 'success' : 'default'}
        />

        {/* Dependent agents count */}
        <StatusIndicator
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 20 20"
              fill="currentColor"
              className="w-4 h-4"
            >
              <path d="M10 9a3 3 0 100-6 3 3 0 000 6zM6 8a2 2 0 11-4 0 2 2 0 014 0zM1.49 15.326a.78.78 0 01-.358-.442 3 3 0 014.308-3.516 6.484 6.484 0 00-1.905 3.959c-.023.222-.014.442.025.654a4.97 4.97 0 01-2.07-.655zM16.44 15.98a4.97 4.97 0 002.07-.654.78.78 0 00.357-.442 3 3 0 00-4.308-3.517 6.484 6.484 0 011.907 3.96 2.32 2.32 0 01-.026.654zM18 8a2 2 0 11-4 0 2 2 0 014 0zM5.304 16.19a.844.844 0 01-.277-.71 5 5 0 019.947 0 .843.843 0 01-.277.71A6.975 6.975 0 0110 18a6.974 6.974 0 01-4.696-1.81z" />
            </svg>
          }
          label={`${session.dependent_agent_count} agent${session.dependent_agent_count !== 1 ? 's' : ''}`}
        />

        {/* Initiation time */}
        <StatusIndicator
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 20 20"
              fill="currentColor"
              className="w-4 h-4"
            >
              <path
                fillRule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zm.75-13a.75.75 0 00-1.5 0v5c0 .414.336.75.75.75h4a.75.75 0 000-1.5h-3.25V5z"
                clipRule="evenodd"
              />
            </svg>
          }
          label={`Established: ${initiatedDate}`}
        />

        {/* OAuth2 Scopes */}
        {session.scope && session.scope.length > 0 && (
          <div className="mt-2">
            <p className="mb-1 text-xs font-medium text-neutral-700">Scopes:</p>
            <Stack direction="row" gap="xs" wrap={true}>
              {session.scope.map((scope) => (
                <Badge key={scope} variant="neutral" size="sm" shape="rounded">
                  {scope}
                </Badge>
              ))}
            </Stack>
          </div>
        )}
      </Stack>
    </Card>
  );
};

export default SessionCard;
