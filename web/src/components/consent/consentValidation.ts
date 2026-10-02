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
  const message = 'message' in error && typeof error.message === 'string' ? error.message : fallback;
  if (!('details' in error) || typeof error.details !== 'object' || error.details === null || Array.isArray(error.details)) return message;
  const fieldErrors = Object.entries(error.details).flatMap(([field, errors]) =>
    Array.isArray(errors) && errors.every((value) => typeof value === 'string') && errors.length > 0
      ? [`${field}: ${errors.join(', ')}`]
      : [],
  );
  return [message, ...fieldErrors].join(' ');
}
