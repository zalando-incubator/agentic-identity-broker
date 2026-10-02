import { useRef, useState } from 'react';
import { useMutation, useMutationState, useQueryClient, type QueryKey } from '@tanstack/react-query';

export interface OptimisticRevokeOptions<TRecord> {
  listKey: QueryKey;
  detailKeys?: (id: string) => readonly QueryKey[];
  recordId: (record: TRecord) => string;
  mutationFn: (id: string) => Promise<unknown>;
  onSuccess?: (id: string) => void;
  onError?: (error: unknown, id: string) => void;
}

export interface OptimisticRevokeResult<TRecord> {
  confirmation: TRecord | null;
  requestRevoke: (record: TRecord) => void;
  cancelRevoke: () => void;
  confirmRevoke: () => Promise<void>;
  isPending: (id: string) => boolean;
  error: unknown;
}

interface RevokeRequest {
  id: string;
}

/** Optimistic pending feedback, with removal only after server acceptance. */
export function useOptimisticRevoke<TRecord>(
  options: OptimisticRevokeOptions<TRecord>,
): OptimisticRevokeResult<TRecord> {
  const client = useQueryClient();
  const [confirmation, setConfirmation] = useState<TRecord | null>(null);
  const confirmationRef = useRef<TRecord | null>(null);
  const [error, setError] = useState<unknown>(null);
  const mutationKey = [...options.listKey, 'revoke'];
  useMutationState({
    filters: { mutationKey, status: 'pending' },
    select: (mutation) => (mutation.state.variables as RevokeRequest).id,
  });

  // Inspect the cache synchronously too: React's next render cannot guard a double click.
  const isInFlight = (id: string) => client.isMutating({
    mutationKey,
    predicate: (mutation) => (mutation.state.variables as RevokeRequest | undefined)?.id === id,
  }) > 0;
  const ownsRequest = (request: RevokeRequest) => client.getMutationCache().find({
    mutationKey,
    status: 'pending',
    predicate: (mutation) => mutation.state.variables === request,
  }) !== undefined;

  const mutation = useMutation({
    mutationKey,
    mutationFn: ({ id }: RevokeRequest) => options.mutationFn(id),
    retry: false,
    networkMode: 'always',
    gcTime: 0,
    onMutate: async ({ id }) => {
      await Promise.all([
        client.cancelQueries({ queryKey: options.listKey }),
        ...(options.detailKeys?.(id) ?? []).map((queryKey) => client.cancelQueries({ queryKey })),
      ]);
      const records = client.getQueryData<TRecord[]>(options.listKey);
      const index = records?.findIndex((record) => options.recordId(record) === id) ?? -1;
      return { index, record: index >= 0 ? records?.[index] : undefined };
    },
    onSuccess: (_data, request) => {
      // A new request for the same record does not own a departed request's settlement.
      if (!ownsRequest(request)) return;
      const { id } = request;
      client.setQueryData<TRecord[]>(options.listKey, (records) =>
        records?.filter((record) => options.recordId(record) !== id),
      );
      options.onSuccess?.(id);
    },
    onError: (failure, request, snapshot) => {
      if (!ownsRequest(request)) return;
      const { id } = request;
      if (snapshot && snapshot.record !== undefined) {
        const record = snapshot.record;
        client.setQueryData<TRecord[]>(options.listKey, (records) => {
          if (!records || records.some((current) => options.recordId(current) === id)) return records;
          const restored = [...records];
          restored.splice(Math.min(snapshot.index, restored.length), 0, record);
          return restored;
        });
      }
      setError(failure);
      options.onError?.(failure, id);
    },
    onSettled: (_data, _failure, request) => {
      if (!ownsRequest(request)) return;
      const { id } = request;
      // The shared MutationCache also refreshes this principal's pending approval count.
      return Promise.all([
        client.invalidateQueries({ queryKey: options.listKey }),
        ...(options.detailKeys?.(id) ?? []).map((queryKey) => client.invalidateQueries({ queryKey })),
      ]);
    },
  });

  function requestRevoke(record: TRecord) {
    if (isInFlight(options.recordId(record))) return;
    confirmationRef.current = record;
    setConfirmation(record);
    setError(null);
  }

  function cancelRevoke() {
    confirmationRef.current = null;
    setConfirmation(null);
  }

  async function confirmRevoke() {
    const record = confirmationRef.current;
    if (record === null) return;
    const id = options.recordId(record);
    cancelRevoke();
    if (isInFlight(id)) return;
    // Failure is exposed by onError and state; an event handler need not catch it again.
    await mutation.mutateAsync({ id }).catch(() => undefined);
  }

  return {
    confirmation,
    requestRevoke,
    cancelRevoke,
    confirmRevoke,
    isPending: isInFlight,
    error,
  };
}
