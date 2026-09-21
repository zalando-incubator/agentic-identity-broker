/**
 * ServiceCard displays a third-party service with connection action.
 *
 * Supports two rendering modes:
 * - Standalone (default): renders with its own Card border
 * - Stacked (inStack=true): renders as a borderless row inside a shared Card,
 *   with isFirst/isLast controlling vertical padding flush to rounded corners
 */

import React from 'react';
import type { ThirdpartyService, DelegatedToken } from '../../types/consent';
import { Card } from '@design-system/components/data-display/Card';
import { Stack } from '@design-system/components/layout/Stack';
import { Button } from '@design-system/components/primitives/Button';
import { Badge } from '@design-system/components/primitives/Badge';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { StatusIndicator } from '@design-system/components/data-display/StatusIndicator';
import { Tooltip } from '@design-system/components/overlays/Tooltip';

interface ServiceCardProps {
  service: ThirdpartyService;
  grants?: DelegatedToken[];
  isLoading?: boolean;
  isDelegated?: boolean;
  onDelegate?: (serviceId: string) => void;
  onRevoke?: (serviceId: string) => void;
  /** When true, renders without own Card border (for use inside a shared stacked card) */
  inStack?: boolean;
  /** First row in stack — gets extra top padding to fill rounded corner */
  isFirst?: boolean;
  /** Last row in stack — gets extra bottom padding to fill rounded corner */
  isLast?: boolean;
}

export function ServiceCard({
  service,
  grants,
  isLoading = false,
  isDelegated = false,
  onDelegate,
  onRevoke,
  inStack = false,
  isFirst = false,
  isLast = false,
}: ServiceCardProps) {
  const serviceDisplayName =
    service.kind === 'scoped'
      ? (service.displayName ?? 'Unknown Service')
      : (service.serviceName ?? 'Unknown Service');

  const serviceScopes =
    service.kind === 'scoped'
      ? (service.scopes ?? []).map((s) => ({ value: s.value, description: s.description }))
      : service.requiredScopes.map((s) => ({ value: s.name, description: s.description }));

  const serviceGrant = grants?.find((g) => g.thirdparty_oauth2_service_id === service.serviceId);
  const grantedScopes = serviceGrant?.scopes || [];

  const isConnected =
    service.kind === 'requirement' && service.connectionStatus === 'connected';

  // Requirement badge — matches PermissionSetCard style
  const requirementBadge = service.requirementType ? (
    <Badge
      variant={service.requirementType === 'mandatory' ? 'primary' : 'neutral'}
      size="sm"
      shape="rounded"
    >
      {service.requirementType === 'mandatory' ? 'Required' : 'Optional'}
    </Badge>
  ) : null;

  const content = (
    <Stack direction="row" gap="md" align="start">
      {/* Avatar */}
      <div className="flex-shrink-0">
        <Avatar
          src={service.logoUrl}
          alt={`${serviceDisplayName} logo`}
          initials={serviceDisplayName.charAt(0).toUpperCase()}
          size="lg"
          shape="rounded"
        />
      </div>

      {/* Service info */}
      <Stack gap="sm" className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <h3 className="text-base font-semibold text-trust-deep">{serviceDisplayName}</h3>
          {requirementBadge}
        </div>

        {isConnected && (
          <StatusIndicator
            label="Active Session"
            variant="success"
            icon={
              <svg className="w-full h-full" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            }
          />
        )}

        {serviceScopes.length > 0 && (
          <Stack gap="xs">
            <span className="text-xs font-semibold text-neutral-500 uppercase tracking-wider">
              Permissions
            </span>
            <div className="flex flex-wrap gap-2">
              {serviceScopes.map((scope) => (
                <Tooltip key={scope.value} content={scope.description || `Scope: ${scope.value}`}>
                  <StatusIndicator
                    label={scope.value}
                    variant={grantedScopes.includes(scope.value) ? 'success' : 'default'}
                    interactive
                  />
                </Tooltip>
              ))}
            </div>
          </Stack>
        )}
      </Stack>

      {/* Action button */}
      {(!isConnected || isDelegated) && (
        <div
          className="flex-shrink-0"
          data-testid={`service-actions-${service.serviceId}`}
        >
          {!isConnected ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => onDelegate?.(service.serviceId)}
              disabled={isLoading}
              title="Connect to this service"
              className="bg-success-primary hover:bg-success-hover text-white"
              data-testid="service-login-button"
            >
              Login
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onRevoke?.(service.serviceId)}
              disabled={isLoading}
              title="Revoke delegation for this service"
              data-testid="service-revoke-button"
            >
              Revoke
            </Button>
          )}
        </div>
      )}
    </Stack>
  );

  if (inStack) {
    // Borderless row inside a shared Card — padding mirrors PermissionSetCard
    return (
      <article
        aria-label={`Service: ${serviceDisplayName}`}
        data-testid={`service-card-${service.serviceId}`}
        className={[
          'px-6',
          isFirst ? 'pt-6' : 'pt-4',
          isLast  ? 'pb-6' : 'pb-4',
        ].join(' ')}
      >
        {content}
      </article>
    );
  }

  // Standalone — own Card border (original behaviour)
  return (
    <article
      aria-label={`Service: ${serviceDisplayName}`}
      data-testid={`service-card-${service.serviceId}`}
    >
      <Card
        padding="none"
        hover="none"
        border="subtle"
        className={isDelegated ? 'ring-2 ring-success-primary bg-success-50' : ''}
      >
        <div className="p-6">{content}</div>
      </Card>
    </article>
  );
}

export default ServiceCard;
