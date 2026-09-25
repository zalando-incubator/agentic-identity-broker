export type ConsentSelections = Record<string, string[]>;

const consentStatePrefix = 'agentic-identity-broker:consent-state:';
const consentStateLifetimeMilliseconds = 15 * 60 * 1000;
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

interface StoredConsentSelections {
  expiresAt: number;
  selections: ConsentSelections;
  serviceID: string;
  returnURL: string;
}

function getSessionStorage(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

function isConsentSelections(value: unknown): value is ConsentSelections {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false;
  }

  return Object.values(value).every(
    (serviceIDs) =>
      Array.isArray(serviceIDs) &&
      serviceIDs.every((id) => typeof id === 'string'),
  );
}

function isStoredConsentSelections(
  value: unknown,
): value is StoredConsentSelections {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false;
  }
  const record = value as Record<string, unknown>;
  return (
    typeof record.expiresAt === 'number' &&
    Number.isFinite(record.expiresAt) &&
    typeof record.serviceID === 'string' &&
    typeof record.returnURL === 'string' &&
    isConsentSelections(record.selections)
  );
}

function pruneExpiredConsentSelections(storage: Storage, now: number): void {
  for (let index = storage.length - 1; index >= 0; index -= 1) {
    const key = storage.key(index);
    if (!key?.startsWith(consentStatePrefix)) {
      continue;
    }

    const rawRecord = storage.getItem(key);
    if (!rawRecord) {
      continue;
    }

    try {
      const record = JSON.parse(rawRecord) as unknown;
      if (
        typeof record !== 'object' ||
        record === null ||
        Array.isArray(record)
      ) {
        continue;
      }

      const expiresAt = (record as Record<string, unknown>).expiresAt;
      if (typeof expiresAt === 'number' && expiresAt <= now) {
        storage.removeItem(key);
      }
    } catch {
      // A malformed record cannot be considered expired.
    }
  }
}

export function saveConsentSelections(
  selections: ConsentSelections,
  serviceID: string,
  returnURL: string,
): string | undefined {
  const storage = getSessionStorage();
  if (!storage) {
    return undefined;
  }

  try {
    const now = Date.now();
    pruneExpiredConsentSelections(storage, now);

    const stateID = crypto.randomUUID();
    storage.setItem(
      `${consentStatePrefix}${stateID}`,
      JSON.stringify({
        expiresAt: now + consentStateLifetimeMilliseconds,
        selections,
        serviceID,
        returnURL,
      }),
    );
    return stateID;
  } catch {
    return undefined;
  }
}

export function loadConsentState(
  stateID: string,
  serviceID: string,
  pathname: string,
): StoredConsentSelections | undefined {
  if (!uuidPattern.test(stateID)) {
    return undefined;
  }

  const storage = getSessionStorage();
  if (!storage) {
    return undefined;
  }

  try {
    const rawRecord = storage.getItem(`${consentStatePrefix}${stateID}`);
    if (!rawRecord) {
      return undefined;
    }

    const record = JSON.parse(rawRecord) as unknown;
    if (
      !isStoredConsentSelections(record) ||
      record.expiresAt <= Date.now() ||
      record.serviceID !== serviceID
    ) {
      return undefined;
    }

    const returnURL = new URL(record.returnURL);
    if (
      returnURL.origin !== window.location.origin ||
      returnURL.pathname !== pathname
    ) {
      return undefined;
    }

    return record;
  } catch {
    return undefined;
  }
}
