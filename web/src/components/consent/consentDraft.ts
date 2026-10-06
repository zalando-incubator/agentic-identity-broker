import { formatISO } from 'date-fns';
import type {
  AgentDetail,
  CreateOrUpdateGrantRequest,
  ResolvedPermissionSetEntry,
  UserGrant,
} from '../../types/consent';

export type GrantDuration = 'until-revoked' | '30-days' | 'custom';
export interface ConsentDraftOptions {
  permissionSets: ResolvedPermissionSetEntry[];
  serviceRequirements?: AgentDetail['service_requirements'];
  existingGrant?: UserGrant | null;
  context: 'decision' | 'console';
  sessionToken?: string;
  restoredDraft?: ConsentDraftSnapshot;
}
export interface DraftService {
  id: string;
  required: boolean;
  selected: boolean;
  readOnly: boolean;
  removalBlocked: boolean;
}
export interface DraftPermissionGroup {
  id: string;
  name: string;
  description: string;
  required: boolean;
  selected: boolean;
  alreadyGranted: boolean;
  readOnly: boolean;
  removalBlocked: boolean;
  collapsed: boolean;
  services: DraftService[];
}
export interface ConsentDraftSnapshot {
  selections: Record<string, string[]>;
  duration: GrantDuration;
  customDate: string;
}
interface DraftDefinition {
  groups: ResolvedPermissionSetEntry[];
  requirements: Map<string, 'mandatory' | 'optional'>;
  prior: Record<string, string[]>;
  context: 'decision' | 'console';
  validUntil?: string | null;
  sessionToken?: string;
}

/** A validation code for the application to translate through its copy catalogue. */
export class InvalidGrantDateError extends Error {
  readonly code = 'invalid-custom-date';
  constructor() {
    super('invalid-custom-date');
    this.name = 'InvalidGrantDateError';
  }
}

function servicesFor(definition: DraftDefinition, entry: ResolvedPermissionSetEntry) {
  return entry.permission_set.service_scopes.filter(({ service_id }) =>
    definition.requirements.size === 0 || definition.requirements.has(service_id),
  );
}

function normalizeSelections(
  definition: DraftDefinition,
  selections: Record<string, string[]>,
): Record<string, string[]> {
  const result: Record<string, string[]> = {};
  const knownIds = new Set(definition.groups.map(({ permission_set }) => permission_set.id));
  for (const [id, services] of Object.entries(definition.prior)) {
    if (definition.context === 'decision' || !knownIds.has(id)) {
      Object.defineProperty(result, id, { value: [...services], enumerable: true, configurable: true, writable: true });
    }
  }
  for (const entry of definition.groups) {
    const id = entry.permission_set.id;
    if (definition.context === 'decision' && Object.prototype.hasOwnProperty.call(definition.prior, id)) continue;
    const selected = Object.prototype.hasOwnProperty.call(selections, id);
    if (!selected && entry.requirement_type !== 'mandatory') continue;
    const chosen = selected ? selections[id]! : undefined;
    const services = servicesFor(definition, entry).filter((service) =>
      service.requirement_type === 'mandatory'
      || definition.requirements.get(service.service_id) === 'mandatory'
      || chosen === undefined
      || chosen.includes(service.service_id),
    ).map(({ service_id }) => service_id);
    Object.defineProperty(result, id, { value: [...new Set(services)], enumerable: true, configurable: true, writable: true });
  }
  return result;
}

/** Immutable local editor state. No transition writes a grant or persists credentials. */
export class ConsentDraft {
  readonly selections: Record<string, string[]>;
  readonly duration: GrantDuration;
  readonly customDate: string;
  readonly sessionToken?: string;

  constructor(
    private readonly definition: DraftDefinition,
    private readonly initial: ConsentDraftSnapshot,
    private readonly state: ConsentDraftSnapshot = initial,
  ) {
    this.selections = state.selections;
    this.duration = state.duration;
    this.customDate = state.customDate;
    this.sessionToken = definition.sessionToken;
  }

  get groups(): DraftPermissionGroup[] {
    const selectedGroupCount = Object.keys(this.selections).length;
    return this.definition.groups.map((entry) => {
      const id = entry.permission_set.id;
      const required = entry.requirement_type === 'mandatory';
      const alreadyGranted = Object.prototype.hasOwnProperty.call(this.definition.prior, id);
      const priorLocked = alreadyGranted && this.definition.context === 'decision';
      const selected = Object.prototype.hasOwnProperty.call(this.selections, id);
      return {
        id,
        name: entry.permission_set.name,
        description: entry.permission_set.description,
        required,
        selected,
        alreadyGranted,
        readOnly: required || priorLocked,
        removalBlocked: selected && selectedGroupCount === 1,
        collapsed: priorLocked,
        services: servicesFor(this.definition, entry).map((service) => {
          const serviceRequired = service.requirement_type === 'mandatory'
            || this.definition.requirements.get(service.service_id) === 'mandatory';
          return {
            id: service.service_id,
            required: serviceRequired,
            selected: selected && this.selections[id]!.includes(service.service_id),
            readOnly: priorLocked || serviceRequired || !selected,
            removalBlocked: selected && this.selections[id]!.length === 1 && this.selections[id]!.includes(service.service_id),
          };
        }),
      };
    });
  }

  get dirty(): boolean {
    if (this.duration !== this.initial.duration || this.customDate !== this.initial.customDate) return true;
    const ids = Object.keys(this.selections);
    return ids.length !== Object.keys(this.initial.selections).length || ids.some((id) => {
      const initialServices = this.initial.selections[id];
      return !initialServices || initialServices.length !== this.selections[id]!.length
        || this.selections[id]!.some((service) => !initialServices.includes(service));
    });
  }

  setPermissionSet(id: string, selected: boolean): ConsentDraft {
    const group = this.groups.find((entry) => entry.id === id);
    if (!group || group.readOnly || group.selected === selected || (!selected && group.removalBlocked)) return this;
    const selections = { ...this.selections };
    if (selected) selections[id] = group.services.map((service) => service.id);
    else delete selections[id];
    return new ConsentDraft(this.definition, this.initial, { ...this.state, selections });
  }

  setService(permissionSetId: string, serviceId: string, selected: boolean): ConsentDraft {
    const group = this.groups.find((entry) => entry.id === permissionSetId);
    const service = group?.services.find((entry) => entry.id === serviceId);
    if (!service || service.readOnly || service.selected === selected || (!selected && service.removalBlocked)) return this;
    const current = this.selections[permissionSetId]!;
    const services = selected ? [...current, serviceId] : current.filter((id) => id !== serviceId);
    return new ConsentDraft(this.definition, this.initial, {
      ...this.state, selections: { ...this.selections, [permissionSetId]: services },
    });
  }

  setDuration(duration: GrantDuration, customDate = this.customDate): ConsentDraft {
    return new ConsentDraft(this.definition, this.initial, { ...this.state, duration, customDate });
  }

  reset(): ConsentDraft {
    return new ConsentDraft(this.definition, this.initial);
  }

  toGrantRequest(now = new Date()): CreateOrUpdateGrantRequest {
    const unchangedValidity = this.duration === this.initial.duration && this.customDate === this.initial.customDate;
    const validUntil = unchangedValidity && this.definition.validUntil
      ? this.definition.validUntil
      : resolveValidUntil(this.duration, this.customDate, now);
    if (unchangedValidity && validUntil && !(Date.parse(validUntil) > now.getTime())) throw new InvalidGrantDateError();
    const grantedPermissionSets = Object.fromEntries(Object.entries(this.selections).map(([id, services]) => [id, [...services]]));
    return validUntil === undefined
      ? { granted_permission_sets: grantedPermissionSets }
      : { granted_permission_sets: grantedPermissionSets, valid_until: validUntil };
  }

  snapshot(): ConsentDraftSnapshot {
    return {
      selections: Object.fromEntries(Object.entries(this.selections).map(([id, services]) => [id, [...services]])),
      duration: this.duration,
      customDate: this.customDate,
    };
  }
}

export function createConsentDraft(options: ConsentDraftOptions): ConsentDraft {
  const existingGrant = options.existingGrant;
  const activeGrant = existingGrant && (existingGrant.valid_until == null || Date.parse(existingGrant.valid_until) > Date.now())
    ? existingGrant : null;
  const definition: DraftDefinition = {
    groups: [
      ...options.permissionSets.filter((entry) => entry.requirement_type === 'mandatory'),
      ...options.permissionSets.filter((entry) => entry.requirement_type === 'optional'),
    ],
    requirements: new Map(options.serviceRequirements?.map((entry) => [entry.service_id, entry.requirement_type])),
    prior: activeGrant?.granted_permission_sets ?? {},
    validUntil: activeGrant?.valid_until,
    context: options.context,
    sessionToken: options.sessionToken,
  };
  const initial: ConsentDraftSnapshot = {
    selections: normalizeSelections(definition, definition.prior),
    duration: definition.validUntil ? 'custom' : 'until-revoked',
    // Custom dates serialize local midnight; restore that calendar date, not the UTC date.
    customDate: definition.validUntil ? formatISO(new Date(definition.validUntil), { representation: 'date' }) : '',
  };
  const restored = options.restoredDraft;
  return new ConsentDraft(definition, initial, restored ? {
    selections: normalizeSelections(definition, restored.selections),
    duration: restored.duration,
    customDate: restored.customDate,
  } : initial);
}

/** Match existing ISO grant timestamps, validating calendar dates before conversion. */
export function resolveValidUntil(duration: GrantDuration, date: string | Date | undefined, now: Date): string | undefined {
  if (duration === 'until-revoked') return undefined;
  if (duration === '30-days') return new Date(now.getTime() + 30 * 86_400_000).toISOString();
  let selected: Date;
  if (date instanceof Date) selected = date;
  else {
    const match = date?.match(/^(\d{4})-(\d{2})-(\d{2})$/);
    if (!match) throw new InvalidGrantDateError();
    const year = Number(match[1]);
    const month = Number(match[2]) - 1;
    const day = Number(match[3]);
    selected = new Date(year, month, day);
    if (selected.getFullYear() !== year || selected.getMonth() !== month || selected.getDate() !== day) throw new InvalidGrantDateError();
  }
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  if (!Number.isFinite(selected.getTime()) || selected < tomorrow) throw new InvalidGrantDateError();
  return selected.toISOString();
}
