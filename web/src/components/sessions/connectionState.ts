import type { SessionSummary } from '@services/api/sessions';
import type { ServiceRequirement } from '../../types/consent';

export type ConnectionStatus = 'no-connection' | 'needs-reauthentication' | 'expired' | 'connected' | 'error';
export interface ConnectionStateInput {
  context: 'sessions' | 'requirement';
  session?: SessionSummary;
  connectionStatus?: ServiceRequirement['connectionStatus'];
  refreshSupported?: boolean;
  readFailed?: boolean;
  now?: Date;
}
export interface ConnectionState {
  status: ConnectionStatus;
  action: 'connect' | 'reconnect' | 'refresh' | 'retry' | null;
  reason?: 'access-expired' | 'refresh-rejected';
  canDisconnect: boolean;
  stale: boolean;
  refetch: boolean;
  rejectedRefresh: boolean;
  refreshSupported: boolean;
}
export type ConnectionEvent =
  | { type: 'refresh-failed'; status?: number }
  | { type: 'read-failed' }
  | { type: 'read-succeeded' | 'refresh-succeeded' | 'reconnected'; session: SessionSummary; now?: Date };

/** Derive only what current API fields prove; scope coverage is deliberately absent. */
export function deriveConnectionState(input: ConnectionStateInput): ConnectionState {
  const { session, context, connectionStatus, readFailed = false, refreshSupported = true, now = new Date() } = input;
  const state: ConnectionState = {
    status: 'error', action: 'retry', canDisconnect: Boolean(session),
    stale: readFailed, refetch: false, rejectedRefresh: false, refreshSupported,
  };
  if (!session && context === 'requirement' && connectionStatus === 'not_connected' && !readFailed) {
    return { ...state, status: 'no-connection', action: 'connect', canDisconnect: false };
  }
  if (!session) {
    // The requirement response itself is authoritative about current usability.
    if (context === 'requirement' && connectionStatus === 'connected' && !readFailed) {
      return { ...state, status: 'connected', action: null };
    }
    return state;
  }
  const refreshExpired = session.has_refresh_token && session.refresh_token_expires_at !== undefined
    && new Date(session.refresh_token_expires_at).getTime() <= now.getTime();
  if (session.is_expired || refreshExpired || (session.access_token_expired && !session.has_refresh_token)) {
    state.status = 'expired';
    state.action = 'reconnect';
  } else if (session.access_token_expired) {
    state.status = 'needs-reauthentication';
    state.action = refreshSupported ? 'refresh' : 'reconnect';
    state.reason = 'access-expired';
  } else {
    state.status = 'connected';
    state.action = session.has_refresh_token && refreshSupported ? 'refresh' : null;
  }
  if (readFailed) state.action = 'retry';
  return state;
}

/** Keep refresh evidence page-local until refresh/reconnect proves new credentials usable. */
export function transitionConnectionState(state: ConnectionState, event: ConnectionEvent): ConnectionState {
  if (event.type === 'read-failed') {
    return { ...state, stale: true, action: 'retry' };
  }
  if (event.type === 'refresh-failed') {
    if (event.status === 409 || event.status === 502) {
      return {
        ...state, status: 'needs-reauthentication', action: 'reconnect', reason: 'refresh-rejected',
        rejectedRefresh: true, stale: false, refetch: false,
      };
    }
    return { ...state, stale: true, refetch: event.status === 404 };
  }
  if (event.type === 'read-succeeded' && state.rejectedRefresh) {
    return { ...state, status: 'needs-reauthentication', action: 'reconnect', stale: false, refetch: false };
  }
  return deriveConnectionState({
    context: 'sessions', session: event.session,
    refreshSupported: state.refreshSupported, now: event.now,
  });
}
