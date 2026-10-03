import { isCancel } from 'axios';
import { isApiError } from '@services/api/client';

export function extractApiError(err: unknown, fallback: string): string {
  if (isCancel(err)) return fallback;
  if (isApiError(err)) return err.message;
  if (err instanceof Error) return err.message;
  return fallback;
}
