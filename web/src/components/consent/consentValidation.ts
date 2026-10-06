import { consentCopy } from '@copy/consent';
import type { ConsentDraft } from './consentDraft';

export function validateDraftSelection(draft: ConsentDraft): string | undefined {
  const selections = Object.values(draft.selections);
  if (draft.groups.length > 0 && selections.length === 0) return consentCopy.selectPermission;
  if (selections.some((services) => services.length === 0)) return consentCopy.selectService;
  return undefined;
}

export function consentErrorMessage(error: unknown, fallback: string): string {
  if (typeof error !== 'object' || error === null) return fallback;
  return 'message' in error && typeof error.message === 'string' ? error.message : fallback;
}
