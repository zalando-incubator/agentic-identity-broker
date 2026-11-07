/**
 * PermissionSetCard — single permission set row inside the shared permissions card.
 *
 * Layout mirrors ServiceCard: Avatar on left, content in middle, toggle on right.
 * SERVICES label is block (above chips), matching PERMISSIONS layout in ServiceCard.
 *
 * FR-008: Within each PS card, only services intersecting with agent's service_requirements
 * are displayed. Mandatory SR services are locked; optional SR services have per-service toggles.
 */

import React, { useMemo } from 'react';
import { Badge } from '@design-system/components/primitives/Badge';
import { Avatar } from '@design-system/components/primitives/Avatar';

export interface ServiceRequirementEntry {
  service_id: string;
  requirement_type: 'mandatory' | 'optional';
}

export interface PermissionSetCardProps {
  permissionSet: {
    id: string;
    name: string;
    description: string;
    service_scopes: Array<{
      service_id: string;
      requirement_type: 'mandatory' | 'optional';
    }>;
  };

  requirementType: 'mandatory' | 'optional';
  isSelected?: boolean;
  onToggle?: (id: string, selected: boolean) => void;
  availableServices?: Array<{ id: string; display_name: string }>;

  /** Agent's service requirements — used to filter and classify services in this PS card (FR-008) */
  serviceRequirements?: ServiceRequirementEntry[];

  /** Which services are currently included within this PS (per-service toggle state) */
  includedServiceIds?: Set<string>;

  /** Called when user toggles an optional SR service within this PS */
  onServiceToggle?: (psId: string, serviceId: string, included: boolean) => void;

  /** Names of requested services this set can cover (Virtual Service Scopes); a marker only */
  requestedServiceNames?: string[];

  isFirst?: boolean;
  isLast?: boolean;
}

export const PermissionSetCard: React.FC<PermissionSetCardProps> = ({
  permissionSet,
  requirementType,
  isSelected = false,
  onToggle,
  availableServices = [],
  serviceRequirements = [],
  includedServiceIds,
  onServiceToggle,
  isFirst = false,
  isLast = false,
  requestedServiceNames = [],
}) => {
  const isMandatory = requirementType === 'mandatory';
  const isActive = isMandatory || isSelected;

  const serviceNameMap = useMemo(
    () => new Map(availableServices.map((s) => [s.id, s.display_name])),
    [availableServices],
  );

  // Build SR requirement-type map: service_id → 'mandatory' | 'optional'
  const srTypeMap = useMemo(
    () => new Map(serviceRequirements.map((sr) => [sr.service_id, sr.requirement_type])),
    [serviceRequirements],
  );

  // Filter service_scopes to only those intersecting with agent's service_requirements (FR-008)
  const displayedServices = useMemo(() => {
    if (serviceRequirements.length === 0) {
      // No SR info available — fall back to showing all services (backward compat)
      return permissionSet.service_scopes.map((ss) => ({
        ...ss,
        srType: undefined as 'mandatory' | 'optional' | undefined,
      }));
    }
    return permissionSet.service_scopes
      .filter((ss) => srTypeMap.has(ss.service_id))
      .map((ss) => ({
        ...ss,
        srType: srTypeMap.get(ss.service_id) as 'mandatory' | 'optional',
      }));
  }, [permissionSet.service_scopes, srTypeMap, serviceRequirements.length]);

  const badgeVariant = isMandatory ? ('primary' as const) : ('neutral' as const);
  const badgeLabel = isMandatory ? 'Required' : 'Optional';

  const handleToggleClick = () => {
    if (onToggle) onToggle(permissionSet.id, !isSelected);
  };

  const handleServiceToggleClick = (serviceId: string, currentlyIncluded: boolean) => {
    if (onServiceToggle) onServiceToggle(permissionSet.id, serviceId, !currentlyIncluded);
  };

  return (
    <div
      data-testid={`permission-set-${isMandatory ? 'mandatory' : 'optional'}`}
      className={[
        'px-6 transition-colors duration-150',
        isFirst ? 'pt-6' : 'pt-4',
        isLast  ? 'pb-6' : 'pb-4',
        isActive ? 'bg-trust-light/20' : 'hover:bg-neutral-50',
      ].join(' ')}
    >
      <div className="flex items-start gap-4">
        {/* Avatar — mirrors ServiceCard layout */}
        <div className="flex-shrink-0">
          <Avatar
            initials={permissionSet.name.charAt(0).toUpperCase()}
            size="lg"
            shape="rounded"
          />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {/* Name + requirement badge */}
          <div className="flex items-center gap-2 flex-wrap mb-1">
            <h3 className="text-base font-semibold text-trust-deep">
              {permissionSet.name}
            </h3>
            <Badge variant={badgeVariant} size="sm" shape="rounded">
              {badgeLabel}
            </Badge>
            {requestedServiceNames.length > 0 && (
              <Badge variant="info" size="sm" shape="rounded" data-testid="requested-candidate-badge">
                Covers requested {requestedServiceNames.join(', ')}
              </Badge>
            )}
          </div>

          {/* Description */}
          <p className="text-sm text-neutral-600 mb-3">
            {permissionSet.description}
          </p>

          {/* Covered services — FR-008: only SR-intersecting services displayed */}
          {displayedServices.length > 0 && (
            <div className="space-y-1">
              <span className="block text-xs font-semibold uppercase tracking-wider text-neutral-500">
                Services
              </span>
              <div className="flex flex-col gap-1.5">
                {displayedServices.map((serviceScope) => {
                  const serviceName =
                    serviceNameMap.get(serviceScope.service_id) ||
                    serviceScope.service_id;
                  const initial = serviceName.charAt(0).toUpperCase();
                  const isServiceMandatory = serviceScope.srType === 'mandatory' || (isActive && serviceScope.requirement_type === 'mandatory');
                  const isServiceIncluded = includedServiceIds
                    ? includedServiceIds.has(serviceScope.service_id)
                    : true; // default to included when no state provided

                  if (isServiceMandatory) {
                    // Mandatory SR service — always locked, always included
                    return (
                      <span
                        key={serviceScope.service_id}
                        className="inline-flex items-center gap-1.5 bg-neutral-100 text-neutral-700 border border-neutral-200 rounded px-1.5 py-0.5 text-xs font-medium w-fit"
                        aria-label={`${serviceName} (required)`}
                      >
                        <Avatar
                          initials={initial}
                          size="xs"
                          shape="circle"
                          className="flex-shrink-0"
                        />
                        {serviceName}
                        <span className="text-neutral-400 text-xs">· required</span>
                      </span>
                    );
                  }

                  // Optional SR service — show with per-service toggle (FR-008)
                  return (
                    <div
                      key={serviceScope.service_id}
                      className={[
                        'inline-flex items-center gap-2 rounded px-2 py-1 text-xs font-medium w-fit',
                        'border transition-colors duration-150',
                        isActive && isServiceIncluded
                          ? 'bg-trust-light/30 border-trust-deep/30 text-trust-deep'
                          : 'bg-neutral-100 border-neutral-200 text-neutral-500',
                      ].join(' ')}
                    >
                      <Avatar
                        initials={initial}
                        size="xs"
                        shape="circle"
                        className="flex-shrink-0"
                      />
                      <span>{serviceName}</span>
                      {isActive && (
                        <button
                          type="button"
                          role="switch"
                          aria-checked={isServiceIncluded}
                          aria-label={`${isServiceIncluded ? 'Exclude' : 'Include'} ${serviceName}`}
                          onClick={() =>
                            handleServiceToggleClick(serviceScope.service_id, isServiceIncluded)
                          }
                          className={[
                            'relative inline-flex h-4 w-7 flex-shrink-0 cursor-pointer rounded-full',
                            'transition-colors duration-200 ease-in-out ml-1',
                            'focus:outline-none focus:ring-2 focus:ring-trust-deep focus:ring-offset-1',
                            isServiceIncluded ? 'bg-trust-deep' : 'bg-neutral-300',
                          ].join(' ')}
                        >
                          <span
                            aria-hidden="true"
                            className={[
                              'pointer-events-none inline-block h-3 w-3 transform rounded-full',
                              'bg-white shadow transition-transform duration-200 ease-in-out',
                              'mt-0.5',
                              isServiceIncluded ? 'translate-x-3.5' : 'translate-x-0.5',
                            ].join(' ')}
                          />
                        </button>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        {/* PS-level toggle — custom implementation uses React state for colors directly,
            avoiding Headless UI data-attribute selector mismatch */}
        {!isMandatory && (
          <button
            type="button"
            role="switch"
            aria-checked={isSelected}
            aria-label={`Toggle ${permissionSet.name}`}
            onClick={handleToggleClick}
            className={[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full',
              'transition-colors duration-200 ease-in-out',
              'focus:outline-none focus:ring-2 focus:ring-trust-deep focus:ring-offset-2',
              isSelected ? 'bg-trust-deep' : 'bg-neutral-300',
            ].join(' ')}
          >
            <span
              aria-hidden="true"
              className={[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full',
                'bg-white shadow-md transition-transform duration-200 ease-in-out',
                'mt-0.5',
                isSelected ? 'translate-x-5' : 'translate-x-0.5',
              ].join(' ')}
            />
          </button>
        )}
      </div>
    </div>
  );
};

export default PermissionSetCard;
