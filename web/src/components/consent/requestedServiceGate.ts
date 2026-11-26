/**
 * Live Save gate for services requested through Virtual Service Scopes
 * (specs/055-incremental-service-authorization, FR-009–FR-011).
 *
 * The server supplies, per requested service, its required scopes, baseline usability and every
 * covering permission set's supplied required scopes and contribution usability, all from one
 * verified session snapshot. This recombines them for the current unsaved selection. The server
 * re-evaluates the submitted selection afresh on Save.
 */

import type { RequestedServiceCoverage, RequestedServiceUsability } from '../../types/consent';

/** Whether the selection turns on covering sets that together supply every required scope */
export type RequestedServiceSelection = 'none' | 'incomplete' | 'on';

export interface RequestedServiceResult {
  serviceId: string;
  serviceName: string;
  satisfied: boolean;
  /** Baseline usability combined with every selected candidate's contribution */
  login: RequestedServiceUsability;
  selection: RequestedServiceSelection;
  missingScopes: string[];
  /** Empty when satisfied; otherwise one sentence naming every unmet condition */
  reason: string;
}

const loginPrecedence: RequestedServiceUsability[] = ['unavailable', 'missing', 'unusable'];

export function evaluateRequestedServices(
  coverage: RequestedServiceCoverage[],
  selection: Record<string, string[]>,
  serviceNames: Record<string, string>,
): RequestedServiceResult[] {
  return coverage.map((service) => {
    const name = serviceNames[service.service_id] ?? service.service_id;

    const selected = service.covering_permission_sets.filter((candidate) =>
      selection[candidate.permission_set_id]?.includes(service.service_id),
    );
    const supplied = new Set(selected.flatMap((candidate) => candidate.supplied_required_scopes));
    const missingScopes =
      selected.length === 0 ? [] : service.required_scopes.filter((scope) => !supplied.has(scope));
    const selectionState: RequestedServiceSelection =
      selected.length === 0 ? 'none' : missingScopes.length > 0 ? 'incomplete' : 'on';

    const usabilities = [service.usability, ...selected.map((candidate) => candidate.usability)];
    const login = loginPrecedence.find((state) => usabilities.includes(state)) ?? 'usable';

    const satisfied = selectionState === 'on' && login === 'usable';
    return {
      serviceId: service.service_id,
      serviceName: name,
      satisfied,
      login,
      selection: selectionState,
      missingScopes,
      reason: satisfied ? '' : unmetSentence(service, name, login, selectionState, missingScopes),
    };
  });
}

function unmetSentence(
  service: RequestedServiceCoverage,
  name: string,
  login: RequestedServiceUsability,
  selection: RequestedServiceSelection,
  missingScopes: string[],
): string {
  if (service.covering_permission_sets.length === 0) {
    return `To save: ask an administrator to add a permission set that allows ${name}.`;
  }
  const steps: string[] = [];
  if (login === 'missing' || login === 'unusable') {
    steps.push(`log in to ${name} under Connect Your Accounts`);
  } else if (login === 'unavailable') {
    steps.push(`retry the ${name} check`);
  }
  if (selection === 'none') {
    steps.push(`turn on a permission set that allows ${steps.length > 0 ? 'it' : name}`);
  } else if (selection === 'incomplete') {
    steps.push(`turn on permission sets that together give ${name} ${missingScopes.join(', ')}`);
  }
  return `To save: ${steps.join(' and ')}.`;
}
