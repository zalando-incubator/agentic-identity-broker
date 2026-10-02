import { createContext, useContext, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { QueryClientProvider, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { consentApi } from '@services/api/consent';
import { advanceAuthGeneration, subscribeAuthLoss } from '@services/api/client';
import type { ApiError, UserInfo } from '../../types/consent';
import { createQueryClient } from './queryClient';

const PrincipalContext = createContext<UserInfo | null>(null);

export interface QueryProviderProps {
  client?: QueryClient;
  children: ReactNode;
  fallback?: ReactNode;
  errorFallback?: (error: unknown) => ReactNode;
}

function IdentityBoundary({
  children,
  fallback = null,
  errorFallback,
}: Omit<QueryProviderProps, 'client'>) {
  const client = useQueryClient();
  const [authError, setAuthError] = useState<ApiError | null>(null);
  const [readyPrincipal, setReadyPrincipal] = useState<string | null>(null);
  const authenticatedPrincipal = useRef<string | null>(null);
  const identity = useQuery({
    queryKey: ['identity'],
    queryFn: async ({ signal }) => {
      const user = await consentApi.getUserInfo({ signal });
      // Fence continuations before Query publishes the new identity to React.
      if (!signal.aborted && authenticatedPrincipal.current !== user.principal) {
        if (authenticatedPrincipal.current !== null) advanceAuthGeneration();
        authenticatedPrincipal.current = user.principal;
      }
      return user;
    },
    enabled: authError === null,
    gcTime: 0,
    refetchOnWindowFocus: 'always',
  });

  useLayoutEffect(() => subscribeAuthLoss((error) => {
    setAuthError(error);
    setReadyPrincipal(null);
    void client.cancelQueries();
    client.removeQueries();
    client.getMutationCache().clear();
  }), [client]);

  const principal = identity.data?.principal;
  useLayoutEffect(() => {
    if (authError || !principal || !identity.isFetchedAfterMount) return;
    let current = true;
    const previous = {
      predicate: (query: { queryKey: readonly unknown[] }) =>
        query.queryKey[0] === 'principal' && query.queryKey[1] !== principal,
    };
    void client.cancelQueries(previous).then(() => {
      if (!current) return;
      client.removeQueries(previous);
      for (const mutation of client.getMutationCache().getAll()) {
        const key = mutation.options.mutationKey;
        if (key?.[0] === 'principal' && key[1] !== principal) {
          client.getMutationCache().remove(mutation);
        }
      }
      setReadyPrincipal(principal);
    });
    return () => { current = false; };
  }, [authError, client, identity.isFetchedAfterMount, principal]);

  const error = authError ?? identity.error;
  if (error && (authError || !identity.data)) return errorFallback?.(error) ?? fallback;
  if (!identity.isFetchedAfterMount || !identity.data || readyPrincipal !== principal) return fallback;
  return (
    <PrincipalContext.Provider key={principal} value={identity.data}>
      {children}
    </PrincipalContext.Provider>
  );
}

export function QueryProvider({ client: providedClient, ...props }: QueryProviderProps) {
  const [client] = useState(() => providedClient ?? createQueryClient());
  return (
    <QueryClientProvider client={client}>
      <IdentityBoundary {...props} />
    </QueryClientProvider>
  );
}

export function usePrincipal(): UserInfo {
  const principal = useContext(PrincipalContext);
  if (!principal) throw new Error('QueryProvider requires an authenticated principal');
  return principal;
}
