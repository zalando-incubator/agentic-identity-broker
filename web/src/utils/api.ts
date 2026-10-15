import { isAxiosError } from 'axios';
import { isApiError } from '@services/api/client';

export function extractApiError(err: unknown, fallback: string): string {
  if (isAxiosError(err)) {
    const msg = err.response?.data?.message;
    return typeof msg === 'string' ? msg : fallback;
  }
  if (err instanceof Error) return err.message;
  if (isApiError(err)) return err.message;
  return fallback;
}
