/**
 * RequestedServiceBanner
 *
 * Explains, for each service the client requested (Virtual Service Scopes), which permission
 * sets can cover it, whether its session is usable, and what still blocks Save. Application
 * component composed from design-system Alert, Badge and Button.
 */

import React from 'react';
import { Alert } from '@design-system/components/feedback/Alert';
import { Badge } from '@design-system/components/primitives/Badge';
import { Button } from '@design-system/components/primitives/Button';
import type { RequestedServiceCoverage, RequestedServiceUsability } from '../../types/consent';
import type { RequestedServiceResult } from './requestedServiceGate';

const usabilityLabel: Record<RequestedServiceUsability, { label: string; variant: 'success' | 'warning' | 'error' | 'neutral' }> = {
  usable: { label: 'Connected', variant: 'success' },
  missing: { label: 'Needs login', variant: 'warning' },
  unusable: { label: 'Needs reconnect', variant: 'error' },
  unavailable: { label: 'Unavailable', variant: 'neutral' },
};

export interface RequestedServiceBannerProps {
  coverage: RequestedServiceCoverage[];
  results: RequestedServiceResult[];
  serviceNames: Record<string, string>;
  permissionSetNames: Record<string, string>;
  onLogin: (serviceId: string) => void;
  onRetry: () => void;
}

export const RequestedServiceBanner: React.FC<RequestedServiceBannerProps> = ({
  coverage,
  results,
  serviceNames,
  permissionSetNames,
  onLogin,
  onRetry,
}) => (
  <div className="space-y-3">
    {coverage.map((service) => {
      const name = serviceNames[service.service_id] ?? service.service_id;
      const result = results.find((r) => r.serviceId === service.service_id);
      const status = usabilityLabel[service.usability];
      const candidates = service.covering_permission_sets.map(
        (candidate) => permissionSetNames[candidate.permission_set_id] ?? candidate.permission_set_id,
      );
      return (
        <div key={service.service_id} data-testid="requested-service-banner">
          <Alert variant={result?.satisfied ? 'success' : 'info'} title={`${name} was requested`}>
            <div className="space-y-2">
              <p className="flex flex-wrap items-center gap-2">
                <span>Session:</span>
                <Badge variant={status.variant} size="sm" data-testid="requested-service-status">
                  {status.label}
                </Badge>
              </p>
              {service.required_scopes.length > 0 && (
                <p>
                  Requires <span className="font-mono">{service.required_scopes.join(', ')}</span>.
                </p>
              )}
              <p>
                {candidates.length > 0
                  ? `Covered by: ${candidates.join(', ')}. Choose the sets you want; nothing is selected for you.`
                  : 'No permission set offered to this agent covers this service. Ask an administrator to update its configuration.'}
              </p>
              {result && !result.satisfied && <p>{result.reason}</p>}
              {(service.usability === 'missing' || service.usability === 'unusable') && (
                <Button variant="secondary" size="sm" onClick={() => onLogin(service.service_id)}>
                  Log in to {name}
                </Button>
              )}
              {service.usability === 'unavailable' && (
                <Button variant="secondary" size="sm" onClick={onRetry}>
                  Retry
                </Button>
              )}
            </div>
          </Alert>
        </div>
      );
    })}
  </div>
);
