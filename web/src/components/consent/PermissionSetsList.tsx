/**
 * PermissionSetsList component displays all permission sets in a single shared card.
 *
 * Groups permission sets by requirement type (mandatory first, then optional).
 * All entries sit inside one Card with <hr> dividers between them.
 *
 * Implements:
 * - FR-007: Render permission sets section
 * - FR-008: Group by requirement_type with mandatory first; per-service toggles for optional SR services
 * - FR-011, FR-012: Selection state management for optional sets and per-service inclusion
 * - FR-014: Lifting selection state to parent component
 */

import React, { useMemo, useState, useEffect, useRef } from 'react';
import { Card } from '@design-system/components/data-display/Card';
import { PermissionSetCard, ServiceRequirementEntry } from './PermissionSetCard';

export interface PermissionSetsListProps {
  /** Array of permission sets with their requirement types */
  permissionSets: Array<{
    permission_set: {
      id: string;
      name: string;
      description: string;
      service_scopes: Array<{
        service_id: string;
        requirement_type: 'mandatory' | 'optional';
      }>;
    };
    requirement_type: 'mandatory' | 'optional';
  }>;

  /** Available services for display name lookup */
  availableServices?: Array<{ id: string; display_name: string }>;

  /** Agent's service requirements — used for per-service toggle classification (FR-008) */
  serviceRequirements?: ServiceRequirementEntry[];

  /**
   * Callback when user selection changes.
   * selectedOptionalIds: currently selected optional PS IDs
   * perPsIncludedServiceIds: map of PS ID → included service IDs (only for active PSes)
   */
  onSelectionChange?: (
    selectedOptionalIds: string[],
    perPsIncludedServiceIds: Record<string, string[]>,
  ) => void;

  /**
   * Pre-hydrate selection state from an existing grant (re-approval scenario).
   * Keys are PS IDs; values are previously-included service IDs.
   * Optional PSes in this map will start selected; per-service state is also restored.
   */
  initialGrantedPermissionSets?: Record<string, string[]>;

  /** Permission set ID → names of requested services it covers (marker only, never a selection) */
  requestedCandidates?: Record<string, string[]>;
}

/**
 * PermissionSetsList renders all permission sets inside one shared Card.
 * Mandatory sets come first; optional sets are managed by local state.
 * Per-service inclusion is tracked for optional SR services within each PS.
 */
export const PermissionSetsList: React.FC<PermissionSetsListProps> = ({
  permissionSets,
  availableServices = [],
  serviceRequirements = [],
  onSelectionChange,
  initialGrantedPermissionSets = {},
  requestedCandidates = {},
}) => {
  const [selectedOptionalIds, setSelectedOptionalIds] = useState<Set<string>>(() => {
    const initial = new Set<string>();
    for (const ps of permissionSets) {
      if (
        ps.requirement_type === 'optional' &&
        initialGrantedPermissionSets[ps.permission_set.id] !== undefined
      ) {
        initial.add(ps.permission_set.id);
      }
    }
    return initial;
  });

  // Per-PS per-service inclusion: psId → Set<serviceId>
  // Tracks user's per-service toggle state within each PS.
  // Initialized when a PS is selected; undefined means "not yet customized" (use defaults).
  const [perPsServiceInclusion, setPerPsServiceInclusion] = useState<Map<string, Set<string>>>(
    () => {
      const initial = new Map<string, Set<string>>();
      for (const [psId, serviceIds] of Object.entries(initialGrantedPermissionSets)) {
        initial.set(psId, new Set(serviceIds));
      }
      return initial;
    },
  );

  // Build SR requirement-type map
  const srTypeMap = useMemo(
    () => new Map(serviceRequirements.map((sr) => [sr.service_id, sr.requirement_type])),
    [serviceRequirements],
  );

  // Compute included service IDs for a given PS (active = mandatory or selected optional)
  const computeIncludedServiceIds = (
    psId: string,
    serviceScopes: Array<{ service_id: string; requirement_type: 'mandatory' | 'optional' }>,
    isActive: boolean,
  ): Set<string> => {
    if (!isActive) return new Set();
    const included = new Set<string>();
    const userInclusion = perPsServiceInclusion.get(psId);
    for (const ss of serviceScopes) {
      const srType = srTypeMap.get(ss.service_id);
      if (!srType && serviceRequirements.length > 0) continue; // Not in SR → skip when SR is known
      // Effectively mandatory: SR mandatory OR ServiceScope mandatory (FR-014).
      // srType === undefined means "no SR for this service" (PS-only agent) — not locked.
      const isEffectivelyMandatory =
        srType === 'mandatory' || ss.requirement_type === 'mandatory';
      if (isEffectivelyMandatory) {
        // Always include — not user-togglable
        included.add(ss.service_id);
      } else {
        // Optional: include if user explicitly included it (or no user state = default on)
        if (userInclusion === undefined || userInclusion.has(ss.service_id)) {
          included.add(ss.service_id);
        }
      }
    }
    return included;
  };

  // Build perPsIncludedServiceIds for parent callback (only for active PSes)
  const buildPerPsIncludedServiceIds = (
    newSelectedOptionalIds: Set<string>,
    newPerPsInclusion: Map<string, Set<string>>,
  ): Record<string, string[]> => {
    const result: Record<string, string[]> = {};
    for (const ps of permissionSets) {
      const psId = ps.permission_set.id;
      const isActive =
        ps.requirement_type === 'mandatory' || newSelectedOptionalIds.has(psId);
      if (!isActive) continue;
      const userInclusion = newPerPsInclusion.get(psId);
      const included = new Set<string>();
      for (const ss of ps.permission_set.service_scopes) {
        const srType = srTypeMap.get(ss.service_id);
        if (!srType && serviceRequirements.length > 0) continue;
        // Effectively mandatory: SR mandatory OR ServiceScope mandatory (FR-014).
        // srType === undefined means "no SR for this service" (PS-only agent) — not locked.
        const isEffectivelyMandatory =
          srType === 'mandatory' || ss.requirement_type === 'mandatory';
        if (isEffectivelyMandatory) {
          included.add(ss.service_id);
        } else {
          if (userInclusion === undefined || userInclusion.has(ss.service_id)) {
            included.add(ss.service_id);
          }
        }
      }
      if (included.size > 0) {
        result[psId] = Array.from(included);
      }
    }
    return result;
  };

  // Separate permission sets by requirement type, mandatory first
  const mandatory = permissionSets.filter((p) => p.requirement_type === 'mandatory');
  const optional = permissionSets.filter((p) => p.requirement_type === 'optional');
  const orderedSets = [...mandatory, ...optional];

  const handleToggle = (id: string, selected: boolean) => {
    const newSelected = new Set(selectedOptionalIds);
    const newInclusion = new Map(perPsServiceInclusion);

    if (selected) {
      newSelected.add(id);
      // Initialize per-service state for this PS if not already set:
      // default all optional SR services to included
      if (!newInclusion.has(id)) {
        const psEntry = permissionSets.find((p) => p.permission_set.id === id);
        if (psEntry) {
          const initialIncluded = new Set<string>();
          for (const ss of psEntry.permission_set.service_scopes) {
            const srType = srTypeMap.get(ss.service_id);
            if (srType) {
              // Include all SR-intersecting services by default
              initialIncluded.add(ss.service_id);
            } else if (serviceRequirements.length === 0) {
              initialIncluded.add(ss.service_id);
            }
          }
          newInclusion.set(id, initialIncluded);
        }
      }
    } else {
      newSelected.delete(id);
      // Retain per-service state so user preferences are preserved on re-select
    }

    setSelectedOptionalIds(newSelected);
    setPerPsServiceInclusion(newInclusion);

    if (onSelectionChange) {
      const perPsIncluded = buildPerPsIncludedServiceIds(newSelected, newInclusion);
      onSelectionChange(Array.from(newSelected), perPsIncluded);
    }
  };

  const handleServiceToggle = (psId: string, serviceId: string, included: boolean) => {
    // Guard: effectively mandatory services cannot be toggled off (FR-014)
    if (!included) {
      const psEntry = permissionSets.find((p) => p.permission_set.id === psId);
      const ss = psEntry?.permission_set.service_scopes.find((s) => s.service_id === serviceId);
      const srType = srTypeMap.get(serviceId);
      if (srType === 'mandatory' || ss?.requirement_type === 'mandatory') return;
    }
    const newInclusion = new Map(perPsServiceInclusion);
    // Seed from current computed state when no saved inclusion exists (e.g. first toggle in mandatory PS)
    let psSet: Set<string>;
    if (newInclusion.has(psId)) {
      psSet = new Set(newInclusion.get(psId));
    } else {
      const psEntry = permissionSets.find((p) => p.permission_set.id === psId);
      const isActive = psEntry?.requirement_type === 'mandatory' || selectedOptionalIds.has(psId);
      psSet = psEntry
        ? computeIncludedServiceIds(psId, psEntry.permission_set.service_scopes, isActive)
        : new Set();
    }
    if (included) {
      psSet.add(serviceId);
    } else {
      psSet.delete(serviceId);
    }
    newInclusion.set(psId, psSet);
    setPerPsServiceInclusion(newInclusion);

    if (onSelectionChange) {
      const perPsIncluded = buildPerPsIncludedServiceIds(selectedOptionalIds, newInclusion);
      onSelectionChange(Array.from(selectedOptionalIds), perPsIncluded);
    }
  };

  // Emit initial hydrated state to parent on mount so perPsIncludedServiceIds is never stale.
  const hasMountEmitted = useRef(false);
  useEffect(() => {
    if (hasMountEmitted.current || !onSelectionChange) return;
    hasMountEmitted.current = true;
    const perPsIncluded = buildPerPsIncludedServiceIds(selectedOptionalIds, perPsServiceInclusion);
    onSelectionChange(Array.from(selectedOptionalIds), perPsIncluded);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Empty state
  if (permissionSets.length === 0) {
    return (
      <p className="text-sm text-neutral-600">
        No permission sets configured for this agent.
      </p>
    );
  }

  return (
    <div>
      {/* Section header + description — outside the card */}
      <h2 className="text-xl font-semibold text-trust-deep">
        Agent Permissions
      </h2>
      <p className="mt-1 mb-4 text-sm text-neutral-600">
        Review the permissions this agent is requesting. Required permissions are
        always granted. Toggle optional permissions on or off based on your preference.
      </p>

      {/* One shared card — padding="none" so rows own all vertical space and
          hover background fills flush to the rounded corners */}
      <Card padding="none" border="subtle" hover="none">
        {orderedSets.map((ps, index) => {
          const isActive =
            ps.requirement_type === 'mandatory' ||
            selectedOptionalIds.has(ps.permission_set.id);
          const includedServiceIds = computeIncludedServiceIds(
            ps.permission_set.id,
            ps.permission_set.service_scopes,
            isActive,
          );

          return (
            <React.Fragment key={ps.permission_set.id}>
              {index > 0 && <hr className="border-neutral-200" />}
              <PermissionSetCard
                permissionSet={ps.permission_set}
                requirementType={ps.requirement_type}
                isSelected={
                  ps.requirement_type === 'mandatory'
                    ? true
                    : selectedOptionalIds.has(ps.permission_set.id)
                }
                onToggle={
                  ps.requirement_type === 'optional' ? handleToggle : undefined
                }
                availableServices={availableServices}
                serviceRequirements={serviceRequirements}
                includedServiceIds={isActive ? includedServiceIds : new Set()}
                onServiceToggle={handleServiceToggle}
                requestedServiceNames={requestedCandidates[ps.permission_set.id]}
                isFirst={index === 0}
                isLast={index === orderedSets.length - 1}
              />
            </React.Fragment>
          );
        })}
      </Card>
    </div>
  );
};

export default PermissionSetsList;
