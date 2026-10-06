import { renderHook } from '@testing-library/react';
import { expect, it } from 'vitest';
import { usePendingCountBump } from './PendingApprovalCount';

it('bumps only for added request IDs after the first successful read', () => {
  const { result, rerender } = renderHook(({ ids, stale }: { ids?: string[]; stale?: boolean }) =>
    usePendingCountBump(ids?.map(id => ({ id })), stale), { initialProps: { ids: undefined as string[] | undefined, stale: false } });

  rerender({ ids: ['first', 'second'], stale: false });
  expect(result.current).toBe(0);
  rerender({ ids: ['second', 'first'], stale: false });
  expect(result.current).toBe(0);
  rerender({ ids: ['first'], stale: false });
  expect(result.current).toBe(0);
  rerender({ ids: ['first', 'third'], stale: true });
  expect(result.current).toBe(0);
  rerender({ ids: ['first', 'third'], stale: false });
  expect(result.current).toBe(1);
  rerender({ ids: ['third', 'first'], stale: false });
  expect(result.current).toBe(1);
  rerender({ ids: ['third'], stale: false });
  rerender({ ids: [], stale: false });
  expect(result.current).toBe(1);
  rerender({ ids: ['fourth'], stale: false });
  expect(result.current).toBe(2);
});
