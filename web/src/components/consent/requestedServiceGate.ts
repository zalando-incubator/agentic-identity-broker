/**
 * Live Save gate for services requested through Virtual Service Scopes
 * (specs/055-incremental-service-authorization, FR-009–FR-011).
 *
 * The server supplies, per requested service, its required scopes, baseline usability and every
 * covering permission set's supplied required scopes and contribution usability, all from one
 * verified session snapshot. This recombines them for the current unsaved selection. The server
 * re-evaluates the submitted selection afresh on Save.
 */

import type { RequestedServiceCoverage } from '../../types/consent';

export interface RequestedServiceResult {
  serviceId: string;
  serviceName: string;
  satisfied: boolean;
  /** Empty when satisfied; otherwise a user-facing explanation */
  reason: string;
}

export function evaluateRequestedServices(
  coverage: RequestedServiceCoverage[],
  selection: Record<string, string[]>,
  serviceNames: Record<string, string>,
): RequestedServiceResult[] {
  return coverage.map((service) => {
    const name = serviceNames[service.service_id] ?? 'the requested service';
    const result = (reason: string): RequestedServiceResult => ({
      serviceId: service.service_id,
      serviceName: name,
      satisfied: reason === '',
      reason,
    });

    const selected = service.covering_permission_sets.filter((candidate) =>
      selection[candidate.permission_set_id]?.includes(service.service_id),
    );
    if (selected.length === 0) {
      return result(`Select a permission set that includes ${name}.`);
    }

    const supplied = new Set(selected.flatMap((candidate) => candidate.supplied_required_scopes));
    const missing = service.required_scopes.filter((scope) => !supplied.has(scope));
    if (missing.length > 0) {
      return result(`${name} also needs ${missing.join(', ')}; select permission sets that together provide it.`);
    }

    const usabilities = [service.usability, ...selected.map((candidate) => candidate.usability)];
    if (usabilities.includes('unavailable')) {
      return result(`${name} could not be verified right now; try again shortly.`);
    }
    if (usabilities.includes('missing')) {
      return result(`Log in to ${name} before saving.`);
    }
    if (usabilities.includes('unusable')) {
      return result(`Reconnect ${name}: the current session cannot provide the selected permissions.`);
    }
    return result('');
  });
}
