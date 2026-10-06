import { approvalQueueCopy as copy } from '@copy/approvalQueue';

export function approvalTimeAgo(timestamp: string, unavailable: string = copy.ageUnknown): string {
  const elapsed = Date.now() - Date.parse(timestamp);
  if (!Number.isFinite(elapsed)) return unavailable;
  const minutes = Math.max(0, Math.floor(elapsed / 60_000));
  if (minutes < 1) return copy.ageSeconds;
  if (minutes < 60) return copy.ageMinutes(minutes);
  const hours = Math.floor(minutes / 60);
  return hours < 24 ? copy.ageHours(hours) : copy.ageDays(Math.floor(hours / 24));
}
