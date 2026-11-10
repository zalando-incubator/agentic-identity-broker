/**
 * SessionCard Component
 *
 * Displays a single OAuth2 session with status information, metadata, and actions.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - Session status badge (Active, Access Token Expired, Login required)
 * - Encryption indicator
 * - Dependent agent count
 * - Session initiation timestamp
 * - OAuth2 scopes display
 * - Refresh and Terminate actions, plus Login once the session requires reauthentication
 * - WCAG 2.1 AA compliant
 */

import React, { useMemo } from 'react';
import { Card } from '@design-system/components/data-display/Card';
import { Button } from '@design-system/components/primitives/Button';
import { Badge } from '@design-system/components/primitives/Badge';
import { StatusIndicator } from '@design-system/components/data-display/StatusIndicator';
import { Stack } from '@design-system/components/layout/Stack';
import type { SessionSummary } from '@services/api/sessions';
import { Clock, Lock, Users } from 'lucide-react';

interface SessionCardProps {
  /** Session data to display */
  session: SessionSummary;
  /** Callback when user clicks terminate button */
  onTerminate: (serviceId: string) => void;
  /** Callback when user clicks view details (optional) */
  onViewDetails?: (serviceId: string) => void;
  /** Callback when user clicks the refresh button (force token refresh) */
  onRefresh?: (serviceId: string) => void;
  /** Callback when user clicks the login button of a session that requires reauthentication */
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
      return { status: 'Login required', variant: 'error' as const };
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

  const needsLogin = session.requires_reauthentication;

  return (
    <Card padding="default" border="subtle" hover="lift">
      {/* Header: Service name and Terminate button */}
      <div className="mb-3 flex items-start justify-between gap-2">
        <div className="flex-1">
          <h3 className="m-0 text-lg font-medium text-neutral-900">
            {session.service_display_name}
          </h3>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="danger"
            size="sm"
            onClick={() => onTerminate(session.service_id)}
            isLoading={loading}
            disabled={loading || refreshing}
            className="mr-2 flex-shrink-0"
          >
            Terminate
          </Button>
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
          {onRefresh && session.has_refresh_token && !needsLogin && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onRefresh(session.service_id)}
              isLoading={refreshing}
              disabled={loading || refreshing}
              className="flex-shrink-0"
            >
              Refresh
            </Button>
          )}
          {onLogin && needsLogin && (
            <Button
              variant="primary"
              size="sm"
              onClick={() => onLogin(session.service_id)}
              disabled={loading}
              className="flex-shrink-0"
            >
              Login
            </Button>
          )}
        </div>
      </div>

      {/* Status */}
      <div className="mb-4 flex items-center justify-between gap-2">
        <Badge variant={variant} showDot>
          {status}
        </Badge>
        {needsLogin && (
          <p className="m-0 text-xs font-medium text-error-primary">
            Log in to reconnect
          </p>
        )}
      </div>

      {/* Body: Session metadata */}
      <Stack gap="sm">
        {/* Encryption indicator */}
        <StatusIndicator
          icon={
            <Lock className="w-4 h-4" aria-hidden="true" strokeWidth={1.5} />
          }
          label={session.is_encrypted ? 'Encrypted' : 'Not Encrypted'}
          variant={session.is_encrypted ? 'success' : 'default'}
        />

        {/* Dependent agents count */}
        <StatusIndicator
          icon={
            <Users className="w-4 h-4" aria-hidden="true" strokeWidth={1.5} />
          }
          label={`${session.dependent_agent_count} agent${session.dependent_agent_count !== 1 ? 's' : ''}`}
        />

        {/* Initiation time */}
        <StatusIndicator
          icon={
            <Clock className="w-4 h-4" aria-hidden="true" strokeWidth={1.5} />
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
