/**
 * ServiceRequirementCard component displays a service requirement with connection status.
 *
 * Displays individual service requirement details including:
 * - Service name with requirement type badge (Required/Optional)
 * - Connection status (Active Session or Login button)
 * - Read-only scope list
 * - Disconnect button for connected services
 *
 * Requirements:
 * - FR-017: Mandatory service visual highlighting (trust-deep badge)
 * - FR-017: Optional service visual styling (neutral badge)
 * - FR-019, FR-020: Connection status display
 * - FR-021: Read-only scope display
 * - FR-022: Scope descriptions
 */

import React from 'react';
import { Card } from '@design-system/components/data-display/Card';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';

export interface ServiceRequirementCardProps {
  /** Unique service identifier */
  serviceId: string;

  /** Service display name */
  serviceName: string;

  /** Whether service is mandatory or optional */
  requirementType: 'mandatory' | 'optional';

  /** List of required scopes for this service */
  requiredScopes: Array<{
    /** OAuth2 scope value (e.g., "read:email") */
    name: string;
    /** Human-readable description of what the scope does */
    description?: string;
  }>;

  /** Current connection status with the service */
  connectionStatus: 'connected' | 'not_connected';

  /** Callback when user clicks Login button */
  onLogin: (serviceId: string) => void;

  /** Callback when user clicks Disconnect button */
  onDisconnect: (serviceId: string) => void;
}

/**
 * ServiceRequirementCard displays a single service requirement with connection status.
 * Used in Phase 6: User Story 3 - Consent Screen Displays Service Requirements.
 */
export const ServiceRequirementCard: React.FC<ServiceRequirementCardProps> = ({
  serviceId,
  serviceName,
  requirementType,
  requiredScopes,
  connectionStatus,
  onLogin,
  onDisconnect,
}) => {
  const isConnected = connectionStatus === 'connected';
  const isMandatory = requirementType === 'mandatory';

  // Determine badge variant based on requirement type
  // FR-017: Mandatory services use trust-deep, optional use neutral
  const badgeVariant = isMandatory
    ? ('primary' as const)
    : ('neutral' as const);
  const badgeLabel = isMandatory ? 'Required' : 'Optional';

  return (
    <Card padding="default" border="subtle" hover="none">
      {/* Header with service name and requirement badge */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <h3 className="text-base font-medium text-trust-deep">
            {serviceName}
          </h3>
          <Badge variant={badgeVariant} size="sm" shape="rounded">
            {badgeLabel}
          </Badge>
        </div>
      </div>

      {/* Connection status section */}
      <div className="mb-4">
        {isConnected ? (
          <Badge variant="success" size="sm" shape="pill">
            Active Session
          </Badge>
        ) : (
          <Button
            variant="primary"
            size="sm"
            onClick={() => onLogin(serviceId)}
          >
            Login
          </Button>
        )}
      </div>

      {/* Required scopes section (read-only) */}
      <div className="space-y-3">
        <p className="text-sm font-medium text-neutral-600">
          Required Permissions:
        </p>
        <div className="space-y-2">
          {requiredScopes.map((scope) => (
            <div
              key={scope.name}
              className="bg-neutral-50 p-3 rounded-md border border-neutral-200"
            >
              <code className="text-sm font-mono font-medium text-trust-deep break-all">
                {scope.name}
              </code>
              {scope.description && (
                <p className="text-xs text-neutral-600 mt-1">
                  {scope.description}
                </p>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Disconnect button for connected services */}
      {isConnected && (
        <div className="mt-4 pt-4 border-t border-neutral-200">
          <Button
            variant="danger"
            size="sm"
            onClick={() => onDisconnect(serviceId)}
          >
            Disconnect
          </Button>
        </div>
      )}
    </Card>
  );
};

export default ServiceRequirementCard;
