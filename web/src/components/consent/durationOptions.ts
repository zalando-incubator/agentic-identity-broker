import { accessCopy, consentCopy } from '@copy';
import type { GrantDuration } from './consentDraft';

export const durationOptions: { value: GrantDuration; label: string }[] = [
  { value: 'until-revoked', label: consentCopy.untilRevoked },
  { value: '30-days', label: accessCopy.thirtyDays },
  { value: 'custom', label: accessCopy.customDate },
];
