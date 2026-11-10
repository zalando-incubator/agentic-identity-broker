/**
 * ServiceRequirementsList component displays grouped service requirements.
 *
 * Groups services by requirement type (mandatory first, then optional).
 * Implements:
 * - FR-016: Render service requirements list
 * - FR-018: Group by requirement_type with mandatory services first
 * - FR-022: Scope descriptions
 */

import React from 'react';
import { ServiceRequirementCard } from './ServiceRequirementCard';
import { Stack } from '@design-system/components/layout/Stack';

export interface ServiceRequirementsListProps {
  /** Array of service requirements to display */
  requirements: Array<{
    /** Unique service identifier */
    serviceId: string;

    /** Service display name */
    serviceName: string;

    /** Whether service is mandatory or optional */
    requirementType: 'mandatory' | 'optional';

    /** List of required scopes for this service */
    requiredScopes: Array<{
      /** OAuth2 scope value */
      name: string;
      /** Human-readable description */
      description?: string;
    }>;

    /** Current connection status with the service */
    connectionStatus: 'connected' | 'not_connected';
  }>;

  /** Callback when user clicks Login button */
  onLogin: (serviceId: string) => void;

  /** Callback when user clicks Disconnect button */
  onDisconnect: (serviceId: string) => void;
}

/**
 * ServiceRequirementsList groups and displays service requirements.
 * Mandatory services are shown first, followed by optional services.
 * Used in Phase 6: User Story 3 - Consent Screen Displays Service Requirements.
 */
export const ServiceRequirementsList: React.FC<
  ServiceRequirementsListProps
> = ({ requirements, onLogin, onDisconnect }) => {
  // Separate requirements into mandatory and optional (FR-018)
  const mandatory = requirements.filter(
    (r) => r.requirementType === 'mandatory',
  );
  const optional = requirements.filter((r) => r.requirementType === 'optional');

  // Empty state
  if (requirements.length === 0) {
    return (
      <p className="text-sm text-neutral-600">
        No service requirements configured for this agent.
      </p>
    );
  }

  return (
    <Stack gap="lg">
      {/* Mandatory Services Section */}
      {mandatory.length > 0 && (
        <div>
          <h3 className="text-sm font-medium text-neutral-700 mb-3">
            Required Services
          </h3>
          <div className="space-y-3">
            {mandatory.map((req) => (
              <ServiceRequirementCard
                key={req.serviceId}
                serviceId={req.serviceId}
                serviceName={req.serviceName}
                requirementType={req.requirementType}
                requiredScopes={req.requiredScopes}
                connectionStatus={req.connectionStatus}
                onLogin={onLogin}
                onDisconnect={onDisconnect}
              />
            ))}
          </div>
        </div>
      )}

      {/* Optional Services Section */}
      {optional.length > 0 && (
        <div>
          <h3 className="text-sm font-medium text-neutral-700 mb-3">
            Optional Services
          </h3>
          <div className="space-y-3">
            {optional.map((req) => (
              <ServiceRequirementCard
                key={req.serviceId}
                serviceId={req.serviceId}
                serviceName={req.serviceName}
                requirementType={req.requirementType}
                requiredScopes={req.requiredScopes}
                connectionStatus={req.connectionStatus}
                onLogin={onLogin}
                onDisconnect={onDisconnect}
              />
            ))}
          </div>
        </div>
      )}
    </Stack>
  );
};

export default ServiceRequirementsList;
