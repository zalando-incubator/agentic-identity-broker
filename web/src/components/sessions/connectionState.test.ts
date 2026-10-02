import { describe, expect, it } from 'vitest';
import type { SessionSummary } from '@services/api/sessions';
import { deriveConnectionState, transitionConnectionState } from './connectionState';

const now = new Date('2026-09-27T12:00:00Z');
const session: SessionSummary = {
  id: 'session', service_id: 'mail', service_display_name: 'Mail', token_type: 'Bearer', scope: ['read'],
  initiated_at: '2026-09-01T00:00:00Z', is_expired: false, access_token_expired: false,
  has_refresh_token: true, dependent_agent_count: 1, is_encrypted: true,
};

describe('connection state precedence', () => {
  it('offers No connection only for a known unconnected agent requirement', () => {
    expect(deriveConnectionState({ context: 'requirement', connectionStatus: 'not_connected', now })).toMatchObject({ status: 'no-connection', action: 'connect' });
    expect(deriveConnectionState({ context: 'sessions', now })).toMatchObject({ status: 'error', action: 'retry' });
    expect(deriveConnectionState({ context: 'requirement', connectionStatus: 'connected', now })).toMatchObject({ status: 'connected' });
    expect(deriveConnectionState({ context: 'requirement', connectionStatus: 'not_connected', session, now })).toMatchObject({ status: 'no-connection' });
  });

  it.each([409, 502])('keeps authoritative refresh rejection %s until a successful refresh, despite later session reads', (status) => {
    const connected = deriveConnectionState({ context: 'sessions', session, now });
    const rejected = transitionConnectionState(connected, { type: 'refresh-failed', status });
    expect(rejected).toMatchObject({ status: 'needs-reauthentication', action: 'reconnect', stale: false });
    const reread = transitionConnectionState(rejected, { type: 'read-succeeded', session, now });
    expect(reread.status).toBe('needs-reauthentication');
    expect(transitionConnectionState(reread, { type: 'refresh-succeeded', session, now })).toMatchObject({ status: 'connected', stale: false });
  });

  it('expires a spent refresh token before treating access as usable', () => {
    expect(deriveConnectionState({ context: 'sessions', session: { ...session, refresh_token_expires_at: now.toISOString() }, now })).toMatchObject({ status: 'expired', action: 'reconnect' });
    expect(deriveConnectionState({ context: 'sessions', session: { ...session, access_token_expired: true, has_refresh_token: false }, now })).toMatchObject({ status: 'expired', action: 'reconnect' });
    expect(deriveConnectionState({ context: 'sessions', session: { ...session, is_expired: true }, now })).toMatchObject({ status: 'expired', action: 'reconnect' });
  });

  it('never labels unrefreshed expired access connected and explains available refresh', () => {
    const expiredAccess = { ...session, access_token_expired: true };
    expect(deriveConnectionState({ context: 'sessions', session: expiredAccess, now })).toMatchObject({ status: 'needs-reauthentication', action: 'refresh', reason: 'access-expired' });
    expect(deriveConnectionState({ context: 'sessions', session: expiredAccess, refreshSupported: false, now })).toMatchObject({ status: 'needs-reauthentication', action: 'reconnect', reason: 'access-expired' });
  });

  it('uses domain usability flags for unknown lifetimes and does not infer missing scopes', () => {
    expect(deriveConnectionState({ context: 'sessions', session: { ...session, scope: [], has_refresh_token: false }, now })).toMatchObject({ status: 'connected', canDisconnect: true });
    expect(deriveConnectionState({ context: 'sessions', session, now })).toMatchObject({ status: 'connected', action: 'refresh', canDisconnect: true });
  });

  it('marks non-authoritative failures stale without changing the last authoritative state', () => {
    const connected = deriveConnectionState({ context: 'sessions', session, now });
    for (const status of [undefined, 0, 400, 500, 503]) {
      expect(transitionConnectionState(connected, { type: 'refresh-failed', status })).toMatchObject({ status: 'connected', stale: true, refetch: false });
    }
    expect(transitionConnectionState(connected, { type: 'read-failed' })).toMatchObject({ status: 'connected', stale: true, action: 'retry' });
    expect(deriveConnectionState({ context: 'sessions', readFailed: true, now })).toMatchObject({ status: 'error', action: 'retry' });
  });

  it('requests a list refresh after 404 without claiming credential rejection', () => {
    const connected = deriveConnectionState({ context: 'sessions', session, now });
    expect(transitionConnectionState(connected, { type: 'refresh-failed', status: 404 })).toMatchObject({ status: 'connected', stale: true, refetch: true });
  });

  it('drops page-local rejection on reload and recomputes after reconnect', () => {
    const input = { context: 'sessions' as const, session: { ...session, access_token_expired: true }, now };
    const rejected = transitionConnectionState(deriveConnectionState(input), { type: 'refresh-failed', status: 409 });
    expect(rejected.action).toBe('reconnect');
    expect(deriveConnectionState(input).action).toBe('refresh');
    expect(transitionConnectionState(rejected, { type: 'reconnected', session, now })).toMatchObject({ status: 'connected', stale: false });
  });
});
