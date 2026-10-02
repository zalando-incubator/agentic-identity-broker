import { afterEach, describe, expect, it, vi } from 'vitest';
import { createQueryClient } from './queryClient';

afterEach(() => vi.restoreAllMocks());

describe('security-safe query retries', () => {
  it.each([400, 401, 403, 404, 409, 410, 422, 429])('never replays a %s read', async (status) => {
    const client = createQueryClient();
    const failure = { status, message: 'rejected' };
    const read = vi.fn().mockRejectedValue(failure);
    await expect(client.fetchQuery({ queryKey: ['principal', 'alice', 'read'], queryFn: read, retryDelay: 0 })).rejects.toBe(failure);
    expect(read).toHaveBeenCalledTimes(1);
    client.clear();
  });

  it('bounds network retries and never retries a security mutation', async () => {
    const client = createQueryClient();
    const failure = { status: 0, message: 'offline' };
    const read = vi.fn().mockRejectedValue(failure);
    await expect(client.fetchQuery({ queryKey: ['principal', 'alice', 'read'], queryFn: read, retryDelay: 0 })).rejects.toBe(failure);
    expect(read).toHaveBeenCalledTimes(3);
    const revoke = vi.fn().mockRejectedValue(failure);
    const mutation = client.getMutationCache().build(client, { mutationFn: revoke, retryDelay: 0 });
    await expect(mutation.execute(undefined)).rejects.toBe(failure);
    expect(revoke).toHaveBeenCalledTimes(1);
    client.clear();
  });
});
