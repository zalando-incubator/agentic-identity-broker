import { focusManager, isCancelledError, MutationCache, QueryClient } from '@tanstack/react-query';
import { isCancel } from 'axios';
import { isApiError } from '@services/api/client';
import { queryKeys } from './queryKeys';

// Query v5's default visibility listener does not cover a visible window regaining focus.
focusManager.setEventListener((notify) => {
  if (typeof window === 'undefined') return;
  const onVisibility = () => notify(!document.hidden);
  const onFocus = () => {
    if (!document.hidden) {
      focusManager.setFocused(undefined);
      notify();
    }
  };
  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('focus', onFocus);
  return () => {
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('focus', onFocus);
  };
});

export function createQueryClient(): QueryClient {
  const client: QueryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => {
          if (isCancelledError(error) || isCancel(error) || (error instanceof Error && error.name === 'AbortError')) return false;
          const status = isApiError(error) ? error.status : undefined;
          return failureCount < 2 && !(typeof status === 'number' && status >= 400 && status < 500);
        },
        retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 5000),
      },
      mutations: {
        retry: false,
        networkMode: 'always',
      },
    },
    mutationCache: new MutationCache({
      onSettled: (_data, _error, _variables, _context, mutation) => {
        const key = mutation.options.mutationKey;
        if (key?.[0] === 'principal' && typeof key[1] === 'string') {
          return client.invalidateQueries({ queryKey: queryKeys.pending(key[1]) });
        }
      },
    }),
  });
  return client;
}
